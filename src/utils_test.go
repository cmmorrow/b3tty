package src

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertToFieldName(t *testing.T) {
	assert.Equal(t, "UserFirstName", convertToFieldName("user-first-name"))
	assert.Equal(t, "Id", convertToFieldName("id"))
	assert.Equal(t, "LongHyphenatedString", convertToFieldName("long-hyphenated-string"))
	assert.Equal(t, "", convertToFieldName(""))
}

func TestSum(t *testing.T) {
	assert.Equal(t, 15, sum([]int{1, 2, 3, 4, 5}))
	assert.Equal(t, 0, sum([]int{}))
	assert.Equal(t, -5, sum([]int{-1, -2, -3, 1}))
	assert.Equal(t, 0, sum([]int{-1, 1}))
}

func TestGenerateToken(t *testing.T) {
	token, err := generateToken(10)
	assert.NoError(t, err)
	assert.Len(t, token, 10)

	token2, err := generateToken(10)
	assert.NoError(t, err)
	assert.NotEqual(t, token, token2)

	emptyToken, err := generateToken(0)
	assert.NoError(t, err)
	assert.Len(t, emptyToken, 0)

	_, err = generateToken(-3)
	assert.Error(t, errors.New("generate token: length is less than zero"))

	longToken, err := generateToken(1000)
	assert.NoError(t, err)
	assert.Len(t, longToken, 1000)
}
