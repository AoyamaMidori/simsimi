package simsimi

import (
	"errors"
	"io"
	"net/url"
	"strconv"
	"sync"
	// "encoding/json"
	"net/http"

	"github.com/json-iterator/go"
)

// URL to be used to make a request
const (
	URLGenerateUUID = "http://www.simsimi.com/getUUID"
	URLRelay        = "http://www.simsimi.com/getRealtimeReq"
)

// Languages supported
const (
	Ko = "ko"
	En = "En"
)

type ID struct {
	UID  int
	UUID string
}

func GenerateID() (*ID, error) {
	resp, err := http.Get(URLGenerateUUID)
	if err != nil {
		return nil, err
	}

	buf, err := absorb(resp.Body)
	if err != nil {
		return nil, err
	}

	i := jsoniter.ParseBytes(buf.Bytes())
	defer bufferPool.Put(buf)

	id := new(ID)
	err = loadObject(i, map[string]interface{}{
		"uid":  &id.UID,
		"uuid": &id.UUID,
	})
	if err != nil {
		return nil, err
	}

	return id, nil
}

type RelayError struct {
	Code     string
	Errno    int
	SQLState string
	Index    int
}

func (err *RelayError) Error() string {
	return err.Code + "(" + strconv.Itoa(err.Errno) + ")"
}

func (id *ID) Relay(text, locale string) (string, error) {
	u, _ := url.Parse(URLRelay)
	u.RawQuery = url.Values{
		"uuid":    []string{id.UUID},
		"reqText": []string{text},
		"lc":      []string{locale},
		"ft":      []string{"1"},
		"status":  []string{"W"},
	}.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return "", err
	}

	buf, err := absorb(resp.Body)
	if err != nil {
		return "", err
	}

	i := jsoniter.ParseBytes(buf.Bytes())
	defer bufferPool.Put(buf)

	if i.WhatIsNext() == jsoniter.String {
		s := i.ReadString()
		if i.Error != nil {
			return "", i.Error
		}
		return "", errors.New("simsimi: unexpected error (locale should be inspected): " + s)
	}

	var status int
	var respText string

	var code string
	var errno int
	var sqlState string
	var index int

	err = loadObject(i, map[string]interface{}{
		"status":       &status,
		"respSentence": &respText,

		"code":     &code,
		"errno":    &errno,
		"sqlState": &sqlState,
		"index":    &index,
	})
	if err != nil {
		return "", err
	}

	if errno != 0 {
		return "", &RelayError{
			Code:     code,
			Errno:    errno,
			SQLState: sqlState,
			Index:    index,
		}
	}

	if status != 200 {
		return "", errors.New("simsimi: responded with status " + strconv.Itoa(status))
	}

	return respText, nil
}

func loadObject(i *jsoniter.Iterator, vars map[string]interface{}) error {
	for {
		key := i.ReadObject()
		if i.Error != nil {
			return i.Error
		}
		if key == "" {
			return nil
		}

		switch v := vars[key].(type) {
		case *int:
			*v = i.ReadInt()
			if i.Error != nil {
				return i.Error
			}

		case *string:
			*v = i.ReadString()
			if i.Error != nil {
				return i.Error
			}

		default:
			i.Skip()
			if i.Error != nil {
				return i.Error
			}
		}
	}
}

type buffer []byte

func (buf *buffer) Write(p []byte) (int, error) {
	*buf = append(*buf, p...)
	return len(p), nil
}

func (buf *buffer) Reset() {
	*buf = (*buf)[:0]
}

func (buf buffer) Bytes() []byte {
	return []byte(buf)
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make(buffer, 0, 1024)
		return &buf
	},
}

func absorb(r io.ReadCloser) (*buffer, error) {
	buf := bufferPool.Get().(*buffer)
	buf.Reset()
	_, err := io.Copy(buf, r)
	if err := r.Close(); err != nil {
		bufferPool.Put(buf)
		return nil, err
	}
	if err != nil {
		bufferPool.Put(buf)
		return nil, err
	}
	return buf, nil
}
