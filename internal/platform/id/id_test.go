package id

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFields(t *testing.T) {
	values, err := ParseFields(
		UUIDField{Key: "user_id", Value: "00000000-0000-0000-0000-000000000001"},
		UUIDField{Key: "organisation_id", Value: "00000000-0000-0000-0000-000000000002"},
	)
	require.NoError(t, err)
	require.Len(t, values, 2)
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", values[0].String())
	assert.Equal(t, "00000000-0000-0000-0000-000000000002", values[1].String())
}

func TestParseFieldsIncludesFailingKey(t *testing.T) {
	_, err := ParseFields(
		UUIDField{Key: "user_id", Value: "00000000-0000-0000-0000-000000000001"},
		UUIDField{Key: "organisation_id", Value: "not-a-uuid"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "organisation_id")
	assert.Contains(t, err.Error(), "not-a-uuid")
}

func TestMustNullUUID(t *testing.T) {
	valid := MustNullUUID("00000000-0000-0000-0000-000000000001", true)
	assert.True(t, valid.Valid)
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", valid.UUID.String())

	invalid := MustNullUUID("", false)
	assert.False(t, invalid.Valid)
	assert.Equal(t, uuid.Nil, invalid.UUID)
}

func TestNullUUIDString(t *testing.T) {
	parsed := MustParse("00000000-0000-0000-0000-000000000001")
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", NullUUIDString(uuid.NullUUID{UUID: parsed, Valid: true}))
	assert.Equal(t, "", NullUUIDString(uuid.NullUUID{UUID: parsed, Valid: false}))
}
