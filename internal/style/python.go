package style

import (
	"io/fs"
	"runtime"
	"strings"
)

func GetPythonCommand() string {
	if runtime.GOOS == "windows" {
		return "python" // Windows не использует python3
	}
	return "python3"
}

// isImageFile проверяет расширение файла
func IsImageFile(file fs.DirEntry) bool {
	name := strings.ToLower(file.Name())
	return strings.HasSuffix(name, ".jpg") ||
		strings.HasSuffix(name, ".jpeg") ||
		strings.HasSuffix(name, ".png")
}
