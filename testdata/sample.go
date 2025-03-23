package sample

import "fmt"

// SampleStruct is used for testing symbol extraction.
type SampleStruct struct {
    FieldOne int
}

// SampleFunc prints a message for testing references.
func SampleFunc() {
    fmt.Println("Hello from SampleFunc")
}

func (s *SampleStruct) Method() {
    fmt.Println("Hello from Method")
}