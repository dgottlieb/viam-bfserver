package util

import (
	"io"
	"io/ioutil"
)

func ResponseBody(reader io.ReadCloser) string {
	defer reader.Close()
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}
