package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// v0.38 — загруженные артефакты (фото, CSV, PDF). Редактор отдаёт файл с
// диска, плагин получает путь к нему строкой в поле.
//
// Почему путь абсолютный, а не относительный. PROTOCOL §1: получив не
// абсолютный путь, плагин склеивает его со своим рабочим каталогом —
// plugins/community/csv_loader/main.py: `if not os.path.isabs(path): path =
// os.path.join(os.getcwd(), path)`. Значит относительный путь из пайплайна
// разрешится в каталоге ПЛАГИНА, и файл из assets/ он не найдёт
// (file_not_found). Поэтому в pipeline.input кладётся абсолютный путь, и
// вместе с ним в пайплайн попадает привязка к машине: переносить такой
// пайплайн на другой компьютер нельзя. Это цена того, что файл вообще
// доступен плагину.

const (
	// maxAssetBytes — потолок на один файл. Без лимита один запрос съест
	// память процесса: multipart целиком разбирается в память.
	maxAssetBytes = 64 << 20 // 64 МиБ
	// maxAssetName — длина имени после очистки. NTFS держит 255 символов на
	// компонент; запас нужен под суффикс -N при коллизии.
	maxAssetName = 96
	// maxAssetList — сколько файлов отдаёт GET.
	maxAssetList = 500
)

// errEmptyAsset — загрузили файл в ноль байт. Отдельная ошибка нужна, чтобы
// отдать 400 (ошибка человека), а не 500 (наша).
var errEmptyAsset = errors.New("файл пустой")

type assetInfo struct {
	Name string `json:"name"`
	// Path абсолютный — см. комментарий выше.
	Path string `json:"path"`
	Size int64  `json:"size"`
	Ext  string `json:"ext,omitempty"`
	MIME string `json:"mime,omitempty"`
	// SavedAt — чтобы в списке видеть, что старше.
	SavedAt time.Time `json:"saved_at"`
}

// handleAssets — GET список загруженного, POST загрузка (multipart, поле "file").
func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listAssets(w)
	case http.MethodPost:
		s.uploadAsset(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listAssets(w http.ResponseWriter) {
	entries, err := os.ReadDir(s.AssetsDir)
	if err != nil && !os.IsNotExist(err) {
		http.Error(w, "assets: "+err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]assetInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(s.AssetsDir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, assetInfo{
			Name:    e.Name(),
			Path:    abs,
			Size:    info.Size(),
			Ext:     strings.TrimPrefix(filepath.Ext(e.Name()), "."),
			MIME:    mime.TypeByExtension(filepath.Ext(e.Name())),
			SavedAt: info.ModTime().UTC(),
		})
	}
	// Свежие сверху: только что загруженный файл — первый в списке.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if len(out) > maxAssetList {
		out = out[:maxAssetList]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) uploadAsset(w http.ResponseWriter, r *http.Request) {
	if err := os.MkdirAll(s.AssetsDir, 0o755); err != nil {
		http.Error(w, "assets: mkdir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Лимит на тело ДО разбора multipart: r.Body надо ограничить, иначе тело
	// уйдёт в память раньше, чем сработает лимит.
	r.Body = http.MaxBytesReader(w, r.Body, maxAssetBytes+(1<<20))
	if err := r.ParseMultipartForm(maxAssetBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, fmt.Sprintf("файл больше %d МиБ", maxAssetBytes>>20),
				http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "нужно поле file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	name, err := sanitizeAssetName(header.Filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := writeAssetExclusive(s.AssetsDir, name, file)
	if err != nil {
		if errors.Is(err, errEmptyAsset) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "assets: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

// writeAssetExclusive — записать содержимое под незанятым именем.
//
// Сначала_final-ное имя резервируется через O_EXCL, потом содержимое пишется
// во временный `<имя>.part` и переименовывается на резерв.
//
// Порядок именно такой. Если проверять занятость только по .part, вторая
// загрузка того же имени пройдёт проверку (первый .part уже переименован и
// освободил путь) и молча перезапишет первый файл — тест
// TestAssetsNameCollisionKeepsBoth ловит именно это. Если писать сразу в
// финальный файл, обрыв загрузки оставит «наполовину загруженный» файл,
// который уедет в пайплайн и упадёт уже на ранe. Поэтому: резерв O_EXCL
// запрещает перезапись, .part + rename не даёт показать полузагруженное.
func writeAssetExclusive(dir, name string, src io.Reader) (assetInfo, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 0; i < 1000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d%s", stem, i, ext)
		}
		// Имя пришло от клиента: проверяем, что оно всё ещё внутри каталога.
		abs, err := secureContainedPath(dir, candidate)
		if err != nil {
			return assetInfo{}, fmt.Errorf("имя %q вне каталога артефактов", candidate)
		}
		// Резервируем финальное имя: O_EXCL здесь — гарантия, что чужой файл
		// не будет перезаписан.
		reserved, err := os.OpenFile(abs, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue // занято — берём следующий суффикс
			}
			return assetInfo{}, err
		}
		if err := reserved.Close(); err != nil {
			_ = os.Remove(abs)
			return assetInfo{}, err
		}

		info, err := fillReserved(abs, candidate, ext, src)
		if err != nil {
			_ = os.Remove(abs) // снимаем резерв, чтобы не занимать имя
			return assetInfo{}, err
		}
		return info, nil
	}
	return assetInfo{}, errors.New("слишком много файлов с таким именем")
}

// fillReserved — записать содержимое в зарезервированный путь через .part.
func fillReserved(abs, name, ext string, src io.Reader) (assetInfo, error) {
	part := abs + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return assetInfo{}, err
	}
	n, copyErr := io.Copy(f, src)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(part)
		if copyErr != nil {
			return assetInfo{}, copyErr
		}
		return assetInfo{}, closeErr
	}
	if n == 0 {
		_ = os.Remove(part)
		return assetInfo{}, errEmptyAsset
	}
	if err := os.Rename(part, abs); err != nil {
		_ = os.Remove(part)
		return assetInfo{}, err
	}
	return assetInfo{
		Name:    name,
		Path:    abs,
		Size:    n,
		Ext:     strings.TrimPrefix(ext, "."),
		MIME:    mime.TypeByExtension(ext),
		SavedAt: time.Now().UTC(),
	}, nil
}

// sanitizeAssetName — имя файла от клиента нельзя брать как есть.
//
// Расширение и основа обрабатываются ОТДЕЛЬНО. Проверка на очищенной целиком
// строке теряла расширение: «моё фото.jpg» превращалась в «jpg» — без точки,
// и mime по нему уже не определить. Сейчас имя, из которого после очистки
// не остаётся ничего разумного, получает нейтральную основу «artifact».
//
// Что и зачем убирается:
//   - путь клиента (../, абсолютный путь, разделители): в каталог артефактов
//     должен попасть файл, а не каталог по выбору клиента;
//   - всё, кроме букв/цифт/точки/дефиса/подчёркивания: в Windows нельзя
//     создать файл с двоеточием, звёздочкой и прочими спецсимволами;
//   - ведущие и хвостовые точки: «.gitignore» — скрытый файл, а «..» —
//     вообще выход из каталога;
//   - имя устройства Windows (CON, NUL, PRN, COM1…): такой файл не создаётся,
//     и каждая следующая попытка съедает один лимит коллизий, то есть имя
//     ведёт себя необъяснимо.
func sanitizeAssetName(raw string) (string, error) {
	base := filepath.Base(filepath.FromSlash(strings.TrimSpace(raw)))
	ext := sanitizeAssetExt(filepath.Ext(base))
	stem := sanitizeStem(strings.TrimSuffix(base, filepath.Ext(base)))
	if stem == "" {
		// Если отброшено всё, включая не-точки, имени нет вовсе: это «..», «.»
		// или пустая строка. Отклоняем — клиент прислал мусор, и называть файл
		// «artifact» значит молча согласиться. Если же имя есть, но съедено
		// очисткой (кириллица, пробелы, эмодзи) — подставляем нейтральную
		// основу: терять загруженное из-за имени нечестно.
		if strings.Trim(base, ". 	") == "" {
			return "", errors.New("имя файла не содержит имени — переименуй файл")
		}
		stem = "artifact"
	}
	name := stem + ext
	if len(name) > maxAssetName {
		// обрезаем основу, расширение не трогаем — иначе перестанем понимать
		// тип файла ровно там, где он и нужен.
		cut := maxAssetName - len(ext)
		if cut < 1 {
			cut = 1
		}
		name = strings.Trim(stem[:cut], "._-") + ext
	}
	if isReservedName(strings.ToUpper(stem)) {
		return "", fmt.Errorf("имя %q зарезервировано системой Windows — переименуй файл", name)
	}
	return name, nil
}

func sanitizeAssetExt(ext string) string {
	var b strings.Builder
	for _, r := range strings.TrimPrefix(ext, ".") {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + ('a' - 'A'))
		} else if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
		if b.Len() >= 16 {
			break
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "." + b.String()
}

func sanitizeStem(stem string) string {
	var b strings.Builder
	// Разделитель откладываем и выписываем только перед следующей буквой:
	// иначе либо получаем «двойной__пробел», либо, наоборот, подчёркивание
	// печатается перед каждой буквой («d_v_o_i_n_o_y»). Пробелов подряд —
	// сколько угодно, подчёркивание выходит одно.
	pendingSep := false
	put := func(s string) {
		if pendingSep {
			b.WriteRune('_')
			pendingSep = false
		}
		b.WriteString(s)
	}
	for _, r := range stem {
		if lat, ok := translit[r]; ok {
			put(lat)
			continue
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-':
			put(string(r))
		case r >= 'A' && r <= 'Z':
			put(string(r + ('a' - 'A')))
		default:
			// пробел и прочие символы — разделитель
			pendingSep = true
		}
	}
	return strings.Trim(b.String(), "._-")
}

// translit — кириллица в латиницу для имён файлов.
//
// Без неё русское имя съедалось целиком и человек получал «artifact.jpg»
// вместо «moe_foto.jpg». Имя артефакта — это то, по чему его потом узнают в
// списке и в пайплайне, поэтому терять его нельзя. Схема практическая
// (ж→zh, щ→shch), как в именах файлов на диске, а не лингвистическая.
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "i", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

func isReservedName(base string) bool {
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) {
		if d := base[3]; d >= '1' && d <= '9' {
			return true
		}
	}
	return false
}
