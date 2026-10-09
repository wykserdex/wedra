//go:build windows

package plugin

import (
	"fmt"
	"os"
	"testing"

	"github.com/wykserdex/wedra/internal/testjob"
)

// TestMain вводит тестовый бинарь в Job Object с KILL_ON_JOB_CLOSE до старта
// любых детей: тогда каждый потомок (python, go run, taskkill) рождается уже
// внутри job и умирает вместе с нами.
//
// Сама логика — в internal/testjob, здесь только вызов. Раньше копия жила в
// этом файле, но тестовых пакетов с порождающими процессы семь, и копии
// разъезжаются: обвязка одна, а пользуются все.
//
// Файл windows-only намеренно: вне Windows job'а нет, делать нечего, и
// TestMain'а на других платформах у пакета не появляется — CI там идёт как
// раньше. Не-Windows ветка пакета testjob нужна пакетам с общим TestMain.
//
// Fail-closed: если job не создался, тесты не запускаем. Иначе прогон пошёл бы
// ровно с той поломкой, ради устранения которой TestMain и нужен, и повис бы
// так же, как без него.
func TestMain(m *testing.M) {
	if err := testjob.Adopt(); err != nil {
		fmt.Fprintf(os.Stderr, "testjob: %v — тесты оставили бы осиротевших детей\n", err)
		os.Exit(1)
	}
	if err := testjob.SelfTest(); err != nil {
		fmt.Fprintf(os.Stderr, "testjob: самопроверка не прошла: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	// Handle жив до самого os.Exit: именно его закрытие ядром и убивает job.
	testjob.KeepAlive()
	os.Exit(code)
}
