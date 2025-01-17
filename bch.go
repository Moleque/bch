// Package bch facilitates  the use of Bose-Chaudhuri-Hocquenghem (BCH) codes and error checking.
// Note that the basis of this package is ported from the example at http://www.eccpage.com/bch3.c.
package bch

import (
	"fmt"
	"math"
)

type cachedTotalSize struct {
	dataLength        int
	correctableErrors int
}

var cachedTotalSizes = make(map[cachedTotalSize]int)

// EncodingConfig stores all the computed values from encoding that are necessary for decoding later.
type EncodingConfig struct {
	// MaxCorrectableErrors is the maximum number of correctable errors allowed by the configuration.
	MaxCorrectableErrors int
	// The length (in bits) of the encoded data.
	CodeLength           int
	// The number of bits of CodeLength that are able to store data.
	StorageBits          int // k
	// The Galois field used in the encoding process.
	field                gField
	// The generator polynomial used in the encoding process.
	generator            [548576]int
	n                    int
	d                    int
}

// ChecksumBits simply returns the number of checksum bits in the config.
func (c EncodingConfig) ChecksumBits() int {
	return c.CodeLength - c.StorageBits
}

// ECCRatio simply returns the ratio of maximum correctable errors to number of bits used.
func (c EncodingConfig) ECCRatio() float64 {
	return float64(c.MaxCorrectableErrors) / float64(c.CodeLength)
}

// String returns the standard notation for a binary BCH code configuration of the EncodingConfig.
func (c EncodingConfig) String() string {
	return fmt.Sprintf("(%d, %d, %d) binary BCH code", c.CodeLength, c.StorageBits, c.d)
}

// gField stores a Galois field for passing between the Encode() and Decode() operations.
type gField struct {
	alphaTo [1048576]int
	indexOf [1048576]int
}

// InvalidInputError is thrown when Encode() is asked to encode with settings that are invalid.
type InvalidInputError struct {
	// Additional information about the problem.
	AdditionalInfo string
}

// Error returns a string that explains the InvalidInputError.
func (e InvalidInputError) Error() string {
	ret := "The argument provided is invalid."
	if len(e.AdditionalInfo) > 0 {
		return fmt.Sprintf("%v Additional info: %v", ret, e.AdditionalInfo)
	}
	return ret
}

// UnachievableConfigError is thrown when Encode() is asked to encode with settings that are either
// useless or unachievable.
type UnachievableConfigError struct {
	// Additional information about the problem.
	AdditionalInfo string
}

// Error returns a string that explains the UnachievableConfigError.
func (e UnachievableConfigError) Error() string {
	ret := "The config asked for is either useless or unachievable."
	if len(e.AdditionalInfo) > 0 {
		return fmt.Sprintf("%v Additional info: %v", ret, e.AdditionalInfo)
	}
	return ret
}

// DataTooCorruptError is thrown when the data provided to Decode() has been corrupted too much to be able
// to recover.
type DataTooCorruptError struct {}

// Error returns a string that explains the DataTooCorruptError.
func (e DataTooCorruptError) Error() string {
	return "The data provided has been corrupted too badly to be able to recover."
}

// CreateConfig creates an EncodingConfig to be re-used for multiple encoding operations with the same configuration.
func CreateConfig(codeLength, correctableErrors int) (config *EncodingConfig, err error) {
	m := int(math.Log2(float64(codeLength))) + 1

	n, p, err := readP(m)
	if err != nil {
		return
	}
	field, err := generateGF(m, n, p)
	if err != nil {
		return
	}
	g, k, d, err := genPoly(correctableErrors, n, codeLength, field)
	if err != nil {
		return
	}

	config = &EncodingConfig{
		MaxCorrectableErrors: correctableErrors,
		CodeLength:           codeLength,
		StorageBits:          k,
		field:                field,
		generator:            g,
		n:                    n,
		d:                    d,
	}

	return
}

// Encode encodes the first k bits of the data in buf based on how many parity bits are required to satisfy
// the maximum correctable number of errors specified by correctableErrors. Note that the maximum correctable
// errors does not scale linearly with the number of parity bits required for the task.
func Encode(config *EncodingConfig, buf *[]uint8) (recd []uint8, err error) {
	//return Encode(cnf.CodeLength, cnf.MaxCorrectableErrors, buf

	data := make([]uint8, config.StorageBits)
	for i := 0; i < config.StorageBits && i < len(*buf); i++ {
		data[i] = (*buf)[i]
	}
	for i := len(*buf); i < config.StorageBits; i++ {
		data[i] = 0
	}

	bb, err := encodeBCH(config.CodeLength, config.StorageBits, config.generator, data)
	if err != nil {
		return
	}

	// Load the data into the code value (ecc bits followed by original data)
	recd = make([]uint8, config.CodeLength)
	for i := 0; i < config.CodeLength - config.StorageBits; i++ {
		recd[i] = uint8(bb[i])
	}
	for i := 0; i < config.StorageBits; i++ {
		recd[i + config.CodeLength - config.StorageBits] = data[i]
	}

	return
}

// Decode decodes a buffer of bits according to an EncodingConfig.
func Decode(config *EncodingConfig, buf *[]uint8) (recd []uint8, errors int, err error) {
	recd, errors, err = decodeBCH(config.CodeLength, config.MaxCorrectableErrors, config.n, config.field, *buf)
	if err != nil {
		return
	}
	recd = recd[config.CodeLength - config.StorageBits:]
	return
}

// StorageBitsForConfig determines the number of bits able to be used for storage with a given configuration.
func StorageBitsForConfig(codeLength, correctableErrors int) (int, error) {
	if codeLength >= 1024 {
		return 0, &UnachievableConfigError{"The specified configuration is beyond the scope of the package."}
	}

	m := int(math.Log2(float64(codeLength))) + 1

	n, p, err := readP(m)
	if err != nil {
		return -1, err
	}
	field, err := generateGF(m, n, p)
	if err != nil {
		return -1, err
	}
	_, k, _, err := genPoly(correctableErrors, n, codeLength, field)
	if err != nil {
		switch err.(type) {
		case UnachievableConfigError, *UnachievableConfigError:
			return 0, nil
		default:
			return k, err
		}
	}

	return k, nil
}

// TotalBitsForConfig is a naïve approach to finding the number of total bits required for a specific number of data
// bits and correctable errors. The function caches it's results.
func TotalBitsForConfig(dataLength, correctableErrors int) (int, error) {
	key := cachedTotalSize{dataLength, correctableErrors}
	if totalSize, ok := cachedTotalSizes[key]; ok {
		return totalSize, nil
	}
	lastVal := -1
	for i := dataLength + 1; true; i++ {
		if testDataLength, err := StorageBitsForConfig(i, correctableErrors); err != nil {
			return -1, err
		} else if testDataLength == dataLength {
			cachedTotalSizes[key] = i
			return i, nil
		} else if lastVal >= 0 && lastVal < dataLength && testDataLength >= dataLength {
			cachedTotalSizes[key] = i
			return i, nil
		} else {
			lastVal = i
		}
	}
	return -1, nil
}

// IsDataCorrupted determines whether or not provided data is corrupted.
func IsDataCorrupted(config *EncodingConfig, data []uint8) bool {
	var i, j, t2 int
	s := make([]int, 1025)

	t2 = 2 * config.MaxCorrectableErrors

	for i = 1; i <= t2; i++ {
		s[i] = 0
		for j = 0; j < config.CodeLength; j++ {
			if data[j] != 0 {
				s[i] ^= config.field.alphaTo[(i * j) % config.n]
			}
		}
		if s[i] != 0 {
			return true
		}
	}

	return false
}


func readP(m int) (n int, p [21]uint8, err error) {
	if m < 2 || m > 20 {
		return n, p,
		InvalidInputError{fmt.Sprintf("The provided m value (%d) is outside the allowed range (%d-%d).",
			m, 2, 20)}
	}

	for i := 1; i < m; i++ {
		p[i] = 0
	}
	p[0], p[m] = 1, 1

	switch m {
	case 2:
		p[1] = 1
	case 3:
		p[1] = 1
	case 4:
		p[1] = 1
	case 5:
		p[2] = 1
	case 6:
		p[1] = 1
	case 7:
		p[3] = 1	// changed index from 1 to 3
	case 8:
		p[4], p[5], p[6] = 1, 1, 1
	case 9:
		p[4] = 1
	case 10:
		p[3] = 1
	case 11:
		p[2] = 1
	case 12:
		p[3], p[4], p[7] = 1, 1, 1
	case 13:
		p[1], p[3], p[4] = 1, 1, 1
	case 14:
		p[1], p[11], p[12] = 1, 1, 1
	case 15:
		p[1] = 1
	case 16:
		p[2], p[3], p[5] = 1, 1, 1
	case 17:
		p[3] = 1
	case 18:
		p[7] = 1
	case 19:
		p[1], p[5], p[6] = 1, 1, 1
	case 20:
		p[3] = 1
	}
	n = 1
	for i := 0; i <= m; i++ {
		n *= 2
	}
	n = n / 2 - 1

	return
}

func generateGF(m, n int, p [21]uint8) (field gField, err error) {
	var alphaTo, indexOf [1048576]int

	mask := 1
	alphaTo[m] = 0
	for i := 0; i < m; i++ {
		alphaTo[i] = mask
		indexOf[alphaTo[i]] = i
		if p[i] != 0 {
			alphaTo[m] ^= mask
		}
		mask <<= 1
	}
	indexOf[alphaTo[m]] = m
	mask >>= 1
	for i := m + 1; i < n; i++ {
		if alphaTo[i - 1] >= mask {
			alphaTo[i] = alphaTo[m] ^ ((alphaTo[i - 1] ^ mask) << 1)
		} else {
			alphaTo[i] = alphaTo[i - 1] << 1
		}
		indexOf[alphaTo[i]] = i
	}
	indexOf[0] = -1

	field = gField{
		alphaTo: alphaTo,
		indexOf: indexOf,
	}

	return
}

func genPoly(t, n, length int, field gField) (g [548576]int, k, d int, err error) {
	if length >= 1024 {
		err = &UnachievableConfigError{"The specified configuration is beyond the scope of the package."}
		return
	}

	var ii, jj, ll, kaux int
	var aux, nocycles, root, noterms, redundancy int
	var test bool
	cycle, size, min, zeroes := make([][]int, 1024), make([]int, 1024), make([]int, 1024), make([]int, 1024)
	for i := 0; i < 1024; i++ {
		cycle[i] = make([]int, 21)
	}

	cycle[0][0] = 0
	size[0] = 1
	cycle[1][0] = 1
	size[1] = 1
	jj = 1

	for do := false; !do || ll < n - 1; do = true {
		// Generate the jj-th cycle set
		ii = 0
		for do := false; !do || aux != cycle[jj][0]; do = true {
			ii++
			cycle[jj][ii] = (cycle[jj][ii - 1] * 2) % n
			size[jj]++
			aux = (cycle[jj][ii] * 2) % n
		}
		// Next cycle set representative
		ll = 0
		for do := false; !do || (test && ll < n - 1); do = true {
			ll++
			test = false
			for ii := 1; ii <= jj && !test; ii++ {
				// Examine previous cycle sets
				for kaux = 0; kaux < size[ii] && !test; kaux++ {
					if ll == cycle[ii][kaux] {
						test = true
					}
				}
			}
		}
		if !test {
			jj++ // Next cycle set index
			cycle[jj][0] = ll
			size[jj] = 1
		}
	}
	nocycles = jj // The number of cycle sets modulo n

	d = 2 * t + 1

	// Search for roots 1, 2, ..., d - 1 in cycle sets
	kaux = 0
	redundancy = 0
	for ii = 1; ii <= nocycles; ii++ {
		min[kaux] = 0
		test = false
		for jj = 0; jj < size[ii] && !test; jj++ {
			for root = 1; root < d && !test; root++ {
				if root == cycle[ii][jj] {
					test = true
					min[kaux] = ii
				}
			}
		}
		if min[kaux] != 0 {
			redundancy += size[min[kaux]]
			kaux++
		}
	}
	noterms = kaux
	kaux = 1
	for ii = 0; ii < noterms; ii++ {
		for jj = 0; jj < size[min[ii]]; jj++ {
			zeroes[kaux] = cycle[min[ii]][jj]
			kaux++
		}
	}

	k = length - redundancy

	if k <= 0 {
		return g, k, d,
		UnachievableConfigError{"With the specified number of recoverable errors and code length, " +
			"no data will be able to be stored."}
	}

	// Compute the generator polynomial
	g[0] = field.alphaTo[zeroes[1]]
	g[1] = 1 // g(x) = (x + zeroes[1]) initially
	for ii = 2; ii <= redundancy; ii++ {
		g[ii] = 1
		for jj = ii - 1; jj > 0; jj-- {
			if g[jj] != 0 {
				g[jj] = g[jj - 1] ^ field.alphaTo[(field.indexOf[g[jj]] + zeroes[ii]) % n]
			} else {
				g[jj] = g[jj - 1]
			}
		}
		g[0] = field.alphaTo[(field.indexOf[g[0]] + zeroes[ii]) % n]
	}

	return
}

// Compute redundancy bb[], the coefficients of b(x). The redundancy
// polynomial b(x) is the remainder after dividing x^(length-k)*data(x)
// by the generator polynomial g(x).
func encodeBCH(length, k int, g [548576]int, data []uint8) (bb [548576]int, err error) {
	var i, j int
	var feedback int

	for i = 0; i < length - k; i++ {
		bb[i] = 0
	}
	for i = k - 1; i >= 0; i-- {
		feedback = int(data[i]) ^ bb[length - k - 1]
		if feedback != 0 {
			for j = length - k - 1; j > 0; j-- {
				if g[j] != 0 {
					bb[j] = bb[j - 1] ^ feedback
				} else {
					bb[j] = bb[j - 1]
				}
			}
			bb[0] = g[0] & feedback
		} else {
			for j = length - k - 1; j > 0; j-- {
				bb[j] = bb[j - 1]
			}
			bb[0] = 0
		}
	}

	return
}

func decodeBCH(length, t, n int, field gField, recd []uint8) ([]uint8, int, error) {
	var i, j, u, q, t2 int
	var count, synError = 0, false
	elp, d, l, uLu, s := make([][]int, 1026), make([]int, 1026), make([]int, 1026), make([]int, 1026), make([]int, 1025)
	for i := 0; i < 1024; i++ {
		elp[i] = make([]int, 1024)
	}
	root, loc, reg := make([]int, 200), make([]int, 200), make([]int, 201)

	t2 = 2 * t

	// First, form the syndromes
	for i = 1; i <= t2; i++ {
		s[i] = 0
		for j = 0; j < length; j++ {
			if recd[j] != 0 {
				s[i] ^= field.alphaTo[(i * j) % n]
			}
		}
		if s[i] != 0 {
			synError = true // Set the error flag if the syndrome is non-zero
		}

		// Convert the syndrome from polynomial to index form
		s[i] = field.indexOf[s[i]]
	}

	// If there are errors, try to correct them
	if synError {
		// Compute the error location polynomial via the Berlekamp
		// iterative algorithm. Following the terminology of Lin and
		// Costello's book :   d[u] is the 'mu'th discrepancy, where
		// u='mu'+1 and 'mu' (the Greek letter!) is the step number
		// ranging from -1 to 2*t (see L&C),  l[u] is the degree of
		// the elp at that step, and u_l[u] is the difference between
		// the step number and the degree of the elp.

		// Initialize table entries
		d[0] = 0 // Index form
		d[1] = s[1] // Index form
		elp[0][0] = 0 // Index form
		elp[1][0] = 1 // Polynomial form
		for i = 1; i < t2; i++ {
			elp[0][i] = -1 // Index form
			elp[1][i] = 0 // Polynomial form
		}
		l[0] = 0
		l[1] = 0
		uLu[0] = -1
		uLu[1] = 0
		u = 0

		for do := false; !do || (u < t2 && l[u + 1] <= t); do = true {
			u++
			if d[u] == -1 {
				l[u + 1] = l[u]
				for i = 0; i <= l[u]; i++ {
					elp[u + 1][i] = elp[u][i]
					elp[u][i] = field.indexOf[elp[u][i]]
				}
			} else {
				// Search for words with greatest u_lu[q] for which d[q] != 0
				q = u - 1
				for d[q] == -1 && q > 0 {
					q--
				}
				// Have found first non-zero d[q]
				if q > 0 {
					j = q
					for do := false; !do || j > 0; do = true {
						j--
						if d[j] != -1 && uLu[q] < uLu[j] {
							q = j
						}
					}
				}

				// Have found q such that d[u] != 0 and u_lu[q] is maximum
				// Store the degree of new elp polynomial
				if l[u] > l[q] + u - q {
					l[u + 1] = l[u]
				} else {
					l[u + 1] = l[q] + u - q
				}

				// Form new elp(x)
				for i = 0; i < t2; i++ {
					elp[u + 1][i] = 0
				}
				for i = 0; i <= l[q]; i++ {
					if elp[q][i] != -1 {
						elp[u + 1][i + u - q] = field.alphaTo[(d[u] + n - d[q] + elp[q][i]) % n]
					}
				}
				for i = 0; i <= l[u]; i++ {
					elp[u + 1][i] ^= elp[u][i]
					elp[u][i] = field.indexOf[elp[u][i]]
				}
			}
			uLu[u + 1] = u - l[u + 1]

			// Form the (u + 1)th discrepancy
			if u < t2 {
				// No discrepancy computed on the last iteration
				if s[u + 1] != -1 {
					d[u + 1] = field.alphaTo[s[u + 1]]
				} else {
					d[u + 1] = 0
				}
				for i = 1; i <= l[u + 1]; i++ {
					if s[u + 1 - i] != -1 && elp[u + 1][i] != 0 {
						d[u + 1] ^= field.alphaTo[(s[u + 1 - i] + field.indexOf[elp[u + 1][i]]) % n]
					}
				}
				// Put d[u + 1] into index form
				d[u + 1] = field.indexOf[d[u + 1]]
			}
		}

		u++
		if l[u] <= t { // Can correct errors
			// Put elp into index form
			for i = 0; i <= l[u]; i++ {
				elp[u][i] = field.indexOf[elp[u][i]]
			}

			// Chien search: find the roots of the error location polynomial
			for i = 1; i <= l[u]; i++ {
				reg[i] = elp[u][i]
			}
			count = 0
			for i = 1; i <= n; i++ {
				q = 1
				for j = 1; j <= l[u]; j++ {
					if reg[j] != -1 {
						reg[j] = (reg[j] + j) % n
						q ^= field.alphaTo[reg[j]]
					}
				}
				// Store the root and error location number indices
				if q == 0 {
					root[count] = i
					loc[count] = n - i
					count++
				}
			}
			if count == l[u] {
				// Number of roots = degree of elp, hence <= t errors
				for i = 0; i < l[u]; i++ {
					recd[loc[i]] ^= 1
				}
			} else {
				return nil, -1, DataTooCorruptError{}
			}
		} else {
			return nil, -1, DataTooCorruptError{}
		}
	}

	return recd[:], count, nil
}
