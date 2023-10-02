package service

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

var openIndenters = 0
var overrideIndenter = true

type Indenter struct {
	prefix    string
	oldStdout *os.File
	newStdout *os.File
	reader    *os.File

	threadActive sync.WaitGroup
}

func NewIndenterWithPrefix(prefix string) io.Closer {
	if overrideIndenter {
		return &Indenter{}
	}

	openIndenters += 1
	var err error
	ret := &Indenter{prefix: prefix}

	ret.reader, ret.newStdout, err = os.Pipe()
	if err != nil {
		panic(err)
	}

	ret.oldStdout = os.Stdout
	os.Stdout = ret.newStdout

	ret.threadActive.Add(1)
	go ret.start()

	return ret
}

func NewIndenter() io.Closer {
	return NewIndenterWithPrefix(strings.Repeat("  ", openIndenters))
}

func (indenter *Indenter) Close() error {
	if overrideIndenter {
		return nil
	}

	openIndenters -= 1

	os.Stdout = indenter.oldStdout
	indenter.newStdout.Close()
	indenter.threadActive.Wait()

	return nil
}

func (indenter *Indenter) start() {
	defer func() {
		indenter.reader.Close()
		indenter.threadActive.Done()
	}()

	scanner := bufio.NewScanner(indenter.reader)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		fmt.Fprintf(indenter.oldStdout, "%s%s\n", indenter.prefix, scanner.Text())
	}

	if scanner.Err() != nil {
		panic(scanner.Err())
	}
}
