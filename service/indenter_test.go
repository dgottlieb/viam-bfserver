package service

import (
	"fmt"
	"testing"
)

func TestIndenter(t *testing.T) {
	fmt.Println("Foo")
	i1 := NewIndenter()
	fmt.Println("Bar")
	i2 := NewIndenter()
	fmt.Println("Baz")
	fmt.Println("UnBaz")
	i2.Close()
	fmt.Println("UnBar")
	i1.Close()
	fmt.Println("UnFoo")
}
