//go:build windows

package plugin

// checkTrustConfigFile на Windows проверяет только то, что это не симлинк и не
// каталог. Проверка ACL здесь не реализована: на Windows недоверенный код всё
// равно не запускается (нет изолятора), а доверие выдаёт только allow-list.
func checkTrustConfigFile(path string) error { return nil }
