package repository

import "errors"

var ErrNotFound = errors.New("record not found")
var ErrAlreadyExists = errors.New("record already exists")
