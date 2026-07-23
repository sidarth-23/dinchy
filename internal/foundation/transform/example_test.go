package transform_test

import (
	"fmt"

	"github.com/sidarth-23/dinchy/internal/foundation/transform"
)

func ExampleApply() {
	value, ok := transform.Apply("trim,lower", " Foo ")
	fmt.Println(value, ok)

	value, ok = transform.Apply("bogus", "x")
	fmt.Println(value, ok)

	// Output:
	// foo true
	// x false
}

func ExampleApplyTo() {
	s := " Foo@Example.COM "
	transform.ApplyTo(transform.SpecEmail, &s)
	fmt.Println(s)

	// Output:
	// foo@example.com
}
