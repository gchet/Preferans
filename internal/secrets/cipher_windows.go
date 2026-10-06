package secrets

import (
	"encoding/base64"
	"unsafe"

	"golang.org/x/sys/windows"
)

type platformCipher struct{}

func Platform() Cipher { return platformCipher{} }

func (platformCipher) Seal(value string) (string, error) {
	b := []byte(value)
	input := windows.DataBlob{Size: uint32(len(b))}
	if len(b) > 0 {
		input.Data = &b[0]
	}
	var output windows.DataBlob
	if err := windows.CryptProtectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	return base64.StdEncoding.EncodeToString(unsafe.Slice(output.Data, output.Size)), nil
}

func (platformCipher) Open(value string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(b) == 0 {
		return "", windows.ERROR_INVALID_DATA
	}
	input := windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
	var output windows.DataBlob
	if err = windows.CryptUnprotectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	return string(unsafe.Slice(output.Data, output.Size)), nil
}
