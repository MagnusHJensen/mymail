package sasl

import (
	"errors"
	"fmt"
	"strings"
)

const UTF8NULL = "\u0000"

const PlainMechanism = "PLAIN"

func PlainAuthMessage(authenticationId, password string) string {
	return fmt.Sprintf("%s%s%s%s", UTF8NULL, authenticationId, UTF8NULL, password)
}

func ParsePlainAuthMessage(plainAuth string) (authorizationId, authenticationId, password string, err error) {
	parts := strings.Split(plainAuth, UTF8NULL)
	if len(parts) != 3 {
		return "", "", "", errors.New("PLAIN data MUST send authentication and password")
	}

	return parts[0], parts[1], parts[2], nil
}
