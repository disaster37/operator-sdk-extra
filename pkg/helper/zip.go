package helper

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"

	"emperror.dev/errors"
	json "github.com/json-iterator/go"
)

const maxZipDecompressedSize = 10 * 1024 * 1024

func ZipAndBase64Encode(originalObject any) (string, error) {
	original, err := json.Marshal(originalObject)
	if err != nil {
		return "", errors.Wrap(err, "Error when convert object to byte sequence")
	}

	// Create a buffer to write our archive to.
	buf := new(bytes.Buffer)

	// Create a new zip archive.
	w := zip.NewWriter(buf)

	f, err := w.Create("original")
	if err != nil {
		return "", err
	}
	_, err = f.Write(original)
	if err != nil {
		return "", err
	}

	// Make sure to check the error on Close.
	err = w.Close()
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func UnZipBase64Decode(original string, originalObject any) error {
	if original == "" {
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(original)
	if err != nil {
		return errors.Wrap(err, "Error when decode original")
	}

	zipReader, err := zip.NewReader(bytes.NewReader(decoded), int64(len(decoded)))
	if err != nil {
		return errors.Wrap(err, "Error when init zip reader")
	}

	// Read the file from zip archive
	if len(zipReader.File) == 0 {
		return errors.New("Error when unzip object: zip archive is empty")
	}
	zipFile := zipReader.File[0]
	unzippedFileBytes, err := readZipFile(zipFile)
	if err != nil {
		return errors.Wrap(err, "Error when unzip object")
	}

	// Convert to object
	if err = json.Unmarshal(unzippedFileBytes, originalObject); err != nil {
		return errors.Wrap(err, "Error when convert byte sequence to object")
	}

	return nil
}

func readZipFile(zf *zip.File) ([]byte, error) {
	f, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // ignore error on purpose
	content, err := io.ReadAll(io.LimitReader(f, maxZipDecompressedSize+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxZipDecompressedSize {
		return nil, fmt.Errorf("zip archive too large: exceeds maximum decompressed size of %d bytes", maxZipDecompressedSize)
	}
	return content, nil
}
