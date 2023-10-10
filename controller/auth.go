package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const PrefixJob = "AURA_JOBKEY_"
const PrefixProject = "AURA_PROJECTKEY_"
const PrefixRunner = "AURA_RUNNERKEY_"

func GenerateFromPassword(pass string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
}

func GenerateRandom(prefix string) (string, []byte, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", nil, err
	}
	pass := base64.RawURLEncoding.EncodeToString(b)[:42]
	pass = prefix + pass
	hash, err := GenerateFromPassword(pass)
	if err != nil {
		return "", nil, err
	}
	return pass, hash, nil
}

func CompareHashAndPassword(hash []byte, pass string) (bool, error) {
	err := bcrypt.CompareHashAndPassword(hash, []byte(pass))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
