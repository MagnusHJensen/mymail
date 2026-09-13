package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	passwordToHash := os.Args[1]

	hashed, err := bcrypt.GenerateFromPassword([]byte(passwordToHash), 12)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Your hashed password is %q\n", hashed)
}
