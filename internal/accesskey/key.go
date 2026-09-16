package accesskey

import (
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

func Validate(key string) error {
	if strings.TrimSpace(key) == "" || utf8.RuneCountInString(key) > 256 || !utf8.ValidString(key) {
		return errors.New("密钥须为 1–256 个有效字符")
	}
	for _, character := range key {
		if unicode.IsControl(character) {
			return errors.New("密钥不能包含控制字符")
		}
	}
	return nil
}
func Read(path string) (string, error) {
	if path == "" {
		return "", errors.New("必须使用 --key-file 配置密钥文件")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("无法打开密钥文件")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("密钥文件不是普通文件")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0007 != 0 {
		return "", errors.New("密钥文件不得允许其他用户访问，请设置权限为 0640 或 0600")
	}
	if info.Size() > 2048 {
		return "", errors.New("密钥文件过大")
	}
	contents, err := io.ReadAll(io.LimitReader(file, 2049))
	if err != nil {
		return "", errors.New("无法读取密钥文件")
	}
	key := strings.TrimRight(string(contents), "\r\n")
	if err = Validate(key); err != nil {
		return "", err
	}
	return key, nil
}
