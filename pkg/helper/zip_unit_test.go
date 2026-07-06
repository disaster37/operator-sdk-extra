package helper

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testData struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestZipAndBase64Encode(t *testing.T) {
	t.Run("nominal case - encode a simple struct", func(t *testing.T) {
		obj := testData{Name: "test", Value: 42}

		encoded, err := ZipAndBase64Encode(obj)
		assert.NoError(t, err)
		assert.NotEmpty(t, encoded)

		// Verify it's actually base64 encoded
		_, err = base64.StdEncoding.DecodeString(encoded)
		assert.NoError(t, err)
	})

	t.Run("encoding struct with nested objects", func(t *testing.T) {
		obj := struct {
			Name string
			Data map[string]interface{}
		}{
			Name: "complex",
			Data: map[string]interface{}{
				"key": "value",
				"num": 123,
			},
		}

		encoded, err := ZipAndBase64Encode(obj)
		assert.NoError(t, err)
		assert.NotEmpty(t, encoded)
	})

	t.Run("error during JSON marshaling", func(t *testing.T) {
		// Create a struct with unmarshable field (like a channel)
		obj := struct {
			Name string
			Chan chan int
		}{
			Name: "test",
			Chan: make(chan int),
		}

		_, err := ZipAndBase64Encode(obj)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Error when convert object to byte sequence")
	})
}

func TestUnZipBase64Decode(t *testing.T) {
	t.Run("nominal case - decode an encoded object", func(t *testing.T) {
		original := testData{Name: "test", Value: 42}

		encoded, err := ZipAndBase64Encode(original)
		assert.NoError(t, err)

		var decoded testData
		err = UnZipBase64Decode(encoded, &decoded)
		assert.NoError(t, err)
		assert.Equal(t, original.Name, decoded.Name)
		assert.Equal(t, original.Value, decoded.Value)
	})

	t.Run("empty string input - should return nil", func(t *testing.T) {
		var decoded testData
		err := UnZipBase64Decode("", &decoded)
		assert.NoError(t, err)
		// decoded should be zero value
		assert.Equal(t, testData{}, decoded)
	})

	t.Run("invalid base64 string", func(t *testing.T) {
		var decoded testData
		err := UnZipBase64Decode("invalid_base64!", &decoded)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Error when decode original")
	})

	t.Run("valid base64 but invalid zip content", func(t *testing.T) {
		// Create a valid base64 string that is not a zip
		invalidZip := base64.StdEncoding.EncodeToString([]byte("not a zip file"))

		var decoded testData
		err := UnZipBase64Decode(invalidZip, &decoded)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Error when init zip reader")
	})

	t.Run("corrupted zip content", func(t *testing.T) {
		// Create a valid base64 encoding of incomplete zip data
		corruptedZipData := []byte{0x50, 0x4B, 0x03, 0x04} // Valid zip header
		encoded := base64.StdEncoding.EncodeToString(corruptedZipData)

		var decoded testData
		err := UnZipBase64Decode(encoded, &decoded)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Error when init zip reader")
	})

	t.Run("zip with no files", func(t *testing.T) {
		// Create an empty zip file
		buf := new(bytes.Buffer)
		w := zip.NewWriter(buf)
		err := w.Close()
		assert.NoError(t, err)

		encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

		var decoded testData
		err = UnZipBase64Decode(encoded, &decoded)
		assert.Error(t, err)
		// Should fail when accessing zipReader.File[0]
	})

	t.Run("invalid JSON in zip", func(t *testing.T) {
		// Create a zip with invalid JSON content
		buf := new(bytes.Buffer)
		w := zip.NewWriter(buf)

		f, err := w.Create("original")
		assert.NoError(t, err)
		_, err = f.Write([]byte("invalid json content"))
		assert.NoError(t, err)

		err = w.Close()
		assert.NoError(t, err)

		encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

		var decoded testData
		err = UnZipBase64Decode(encoded, &decoded)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Error when convert byte sequence to object")
	})

	t.Run("valid zip with valid JSON but wrong type", func(t *testing.T) {
		original := "just a string"

		encoded, err := ZipAndBase64Encode(original)
		assert.NoError(t, err)

		var decoded testData // expecting struct, getting string
		err = UnZipBase64Decode(encoded, &decoded)
		// This might succeed or fail depending on json unmarshaling behavior
		// The important thing is that if it fails, it has the right error message
		if err != nil {
			assert.Contains(t, err.Error(), "Error when convert byte sequence to object")
		}
	})
}

func TestReadZipFile(t *testing.T) {
	t.Run("read a valid zip file", func(t *testing.T) {
		content := "test content"

		buf := new(bytes.Buffer)
		w := zip.NewWriter(buf)

		f, err := w.Create("test.txt")
		assert.NoError(t, err)
		_, err = f.Write([]byte(content))
		assert.NoError(t, err)

		err = w.Close()
		assert.NoError(t, err)

		zipReader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		assert.NoError(t, err)

		zipFile := zipReader.File[0]
		data, err := readZipFile(zipFile)
		assert.NoError(t, err)
		assert.Equal(t, content, string(data))
	})

	t.Run("error opening zip file (e.g. corrupt file)", func(t *testing.T) {
		buf := new(bytes.Buffer)
		w := zip.NewWriter(buf)

		f, err := w.Create("test.txt")
		require.NoError(t, err)
		_, err = f.Write([]byte("content"))
		require.NoError(t, err)

		err = w.Close()
		require.NoError(t, err)

		raw := buf.Bytes()
		// Corrupt the compressed data portion to cause Open to fail
		if len(raw) > 39 {
			raw[39] ^= 0xFF
		}

		zipReader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		require.NoError(t, err)

		zipFile := zipReader.File[0]
		_, err = readZipFile(zipFile)
		assert.Error(t, err)
	})

	t.Run("oversized decompression", func(t *testing.T) {
		buf := new(bytes.Buffer)
		w := zip.NewWriter(buf)

		f, err := w.Create("large.txt")
		assert.NoError(t, err)
		_, err = f.Write([]byte(strings.Repeat("a", maxZipDecompressedSize+1)))
		assert.NoError(t, err)

		err = w.Close()
		assert.NoError(t, err)

		zipReader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		assert.NoError(t, err)

		zipFile := zipReader.File[0]
		_, err = readZipFile(zipFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too large")
	})
}
