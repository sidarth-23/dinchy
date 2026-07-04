package store

import "github.com/google/uuid"

func taskIDForName(taskName string) string {
	return uuid.NewSHA1(uuid.Nil, []byte(taskName)).String()
}
