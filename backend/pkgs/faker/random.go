// Package faker provides a simple interface for generating fake data for testing.
package faker

import (
	"math/rand"
	"time"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

type Faker struct{}

func NewFaker() *Faker {
	return &Faker{}
}

func (f *Faker) Time() time.Time {
	return time.Now().Add(time.Duration(f.Num(1, 100)) * time.Hour)
}

func (f *Faker) Str(length int) string {
	b := make([]rune, length)
	for i := range b {
		// #nosec G404 -- Faker generates non-security test fixtures only.
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func (f *Faker) Path() string {
	return "/" + f.Str(10) + "/" + f.Str(10) + "/" + f.Str(10)
}

func (f *Faker) Email() string {
	return f.Str(10) + "@example.com"
}

func (f *Faker) Bool() bool {
	// #nosec G404 -- Faker generates non-security test fixtures only.
	return rand.Intn(2) == 1
}

func (f *Faker) Num(min, max int) int {
	// #nosec G404 -- Faker generates non-security test fixtures only.
	return rand.Intn(max-min) + min
}
