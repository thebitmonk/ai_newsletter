package auth

import "golang.org/x/crypto/bcrypt"

const bcryptCost = 12

func HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(plaintext, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
