package domain

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	resourceIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	portCodePattern   = regexp.MustCompile(`^[A-Z]{5}$`)
	imoPattern        = regexp.MustCompile(`^IMO[0-9]{7}$`)
	containerPattern  = regexp.MustCompile(`^[A-Z]{3}U[0-9]{7}$`)
)

func NormalizeResourceID(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 3 || len(value) > 64 || !resourceIDPattern.MatchString(value) {
		return "", fmt.Errorf("%w: resource id", ErrInvalid)
	}
	return value, nil
}

func NormalizePortCode(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if !portCodePattern.MatchString(value) {
		return "", fmt.Errorf("%w: UN/LOCODE", ErrInvalid)
	}
	return value, nil
}

func NormalizeIMO(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if !imoPattern.MatchString(value) {
		return "", fmt.Errorf("%w: vessel IMO", ErrInvalid)
	}
	return value, nil
}

func NormalizeContainerNumber(value string) (string, error) {
	value = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	if !containerPattern.MatchString(value) || !validContainerCheckDigit(value) {
		return "", fmt.Errorf("%w: container number", ErrInvalid)
	}
	return value, nil
}

func validContainerCheckDigit(value string) bool {
	if len(value) != 11 {
		return false
	}
	letterValue := func(letter byte) int {
		value := int(letter-'A') + 10
		value += (value - 1) / 10
		return value
	}
	total := 0
	for index := 0; index < 10; index++ {
		valueAtIndex := 0
		if value[index] >= '0' && value[index] <= '9' {
			valueAtIndex = int(value[index] - '0')
		} else {
			valueAtIndex = letterValue(value[index])
		}
		total += valueAtIndex * (1 << index)
	}
	check := total % 11
	if check == 10 {
		check = 0
	}
	return check == int(value[10]-'0')
}
