package id_test

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/foundation/id"
)

func ExampleParse() {
	parsed, err := id.Parse("018f2c8d-1a2b-7c3d-8e4f-000000000001")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(parsed.String())
	// Output:
	// 018f2c8d-1a2b-7c3d-8e4f-000000000001
}

func ExampleMustParse() {
	parsed := id.MustParse("018f2c8d-1a2b-7c3d-8e4f-000000000002")
	fmt.Println(parsed.String())
	// Output:
	// 018f2c8d-1a2b-7c3d-8e4f-000000000002
}

func ExampleNullUUIDString() {
	valid, err := id.NullUUID("018f2c8d-1a2b-7c3d-8e4f-000000000003", true)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%q\n", id.NullUUIDString(valid))
	fmt.Printf("%q\n", id.NullUUIDString(uuid.NullUUID{}))
	// Output:
	// "018f2c8d-1a2b-7c3d-8e4f-000000000003"
	// ""
}

func ExampleParseFields() {
	values, err := id.ParseFields(
		id.UUIDField{Key: "tenant", Value: "018f2c8d-1a2b-7c3d-8e4f-000000000004"},
		id.UUIDField{Key: "user", Value: "018f2c8d-1a2b-7c3d-8e4f-000000000005"},
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(len(values))
	fmt.Println(values[0].String())
	fmt.Println(values[1].String())
	// Output:
	// 2
	// 018f2c8d-1a2b-7c3d-8e4f-000000000004
	// 018f2c8d-1a2b-7c3d-8e4f-000000000005
}
