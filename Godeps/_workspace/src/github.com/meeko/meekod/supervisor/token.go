// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package supervisor

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

const (
	TokenFile = ".meeko_token"
	TokenLen  = 32
)

func ReadOrGenerateToken(dir string) ([]byte, error) {
	path := filepath.Join(dir, TokenFile)
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			token := make([]byte, TokenLen)
			n, err := io.ReadFull(rand.Reader, token)
			if err != nil {
				return nil, err
			}

			tokenHex := make([]byte, hex.EncodedLen(n))
			hex.Encode(tokenHex, token)

			err = ioutil.WriteFile(path, tokenHex, 0600)
			if err != nil {
				return nil, err
			}

			return token, nil
		}
		return nil, err
	}

	token, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return token, nil
}
