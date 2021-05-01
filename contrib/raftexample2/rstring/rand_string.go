/*
	The rstring package contains only one function, which generates a random string in a fast way. Possible characters
	of the string are from a to z and from A to Z.

	Note:

	1. Random strings are used as the primary key of clients' read and write requests to the database.
	2. The random string generation function follows the RandStringBytesMaskImprSrcUnsafe() function
	at https://stackoverflow.com/a/31832326
*/
package rstring

import (
	"math/rand"
	"unsafe"
)

const (
	letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" // low conflict
	//letterBytes   = "abcde" // high conflict
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

/*
	Generates a random string

	src: a pointer to a rand.Rand object, so this function can be called by multiple Goroutines
	n: the length of the desired random string, including characters from a to z and A tot Z.

	It returns the generated string
*/
func RandString(src *rand.Rand, n int) string {
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return *(*string)(unsafe.Pointer(&b))
}
