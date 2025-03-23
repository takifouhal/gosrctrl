package sample

// TestInterface defines a simple interface to test implementation detection
type TestInterface interface {
	DoSomething() string
}

// TestImplementor is a struct that implements TestInterface
type TestImplementor struct {
	Value string
}

// DoSomething implements TestInterface
func (t *TestImplementor) DoSomething() string {
	return t.Value
}

// EmbeddingStruct embeds TestImplementor to test embedding detection
type EmbeddingStruct struct {
	TestImplementor // Embedded struct
	ExtraField string
}

// EmbeddingInterface embeds TestInterface to test interface embedding
type EmbeddingInterface interface {
	TestInterface // Embedded interface
	ExtraMethod() int
}