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

// MultiLevelEmbed demonstrates multi-level embedding
type MultiLevelEmbed interface {
	EmbeddingInterface // Embedded interface that itself embeds another interface
	DeepMethod() bool
}

// IndirectImplementor doesn't directly implement any interface
// but inherits implementation from an embedded struct
type IndirectImplementor struct {
	TestImplementor // Will inherit DoSomething method
	IndirectField string
}

// ComplexEmbed demonstrates a struct that embeds multiple types
type ComplexEmbed struct {
	TestImplementor  // Embedded struct
	*EmbeddingStruct // Embedded pointer to struct
	DirectField string
}