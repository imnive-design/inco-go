package example

import "fmt"

// --- Generics: type-aware contract assertions for type parameters ---

// Number is a constraint for numeric types.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// --- Case 1: Comparable type param — generates v == *new(T) ---

// FirstNonZero returns the first non-zero-valued element from a slice.
// The comparable constraint allows inco to use == *new(T) for the zero check.
func FirstNonZero[T comparable](items []T) (result T) {
	// @require -nd result
	for _, v := range items {
		var zero T
		if v != zero {
			return v
		}
	}
	return // will trigger ensure violation if all items are zero-valued
}

// --- Case 2: Numeric type param (also comparable) ---

// Sum requires a non-zero initial value and returns the sum.
func Sum[N Number](initial N, values []N) N {
	// @require -nd initial
	result := initial
	for _, v := range values {
		result += v
	}
	return result
}

// --- Case 3: Any type param (non-comparable) — generates reflect.ValueOf check ---

// MustNotBeZero panics if the given value is the zero value of its type.
// Since T is constrained to `any`, the check uses reflect.
func MustNotBeZero[T any](v T) T {
	// @require -nd v
	return v
}

// --- Case 4: Mixed — comparable and non-comparable type params together ---

// Pair holds two values of potentially different type param constraints.
type Pair[K comparable, V any] struct {
	Key   K
	Value V
}

// NewPair creates a Pair, ensuring the key is not zero-valued.
func NewPair[K comparable, V any](key K, value V) Pair[K, V] {
	// @require -nd key
	return Pair[K, V]{Key: key, Value: value}
}

// --- Case 5: Expression mode with type params ---

// Clamp constrains a value to [lo, hi] with expression-based preconditions.
func Clamp[N Number](val, lo, hi N) N {
	// @require lo <= hi, "lo must not exceed hi"
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}

// --- Case 6: @must with generic return ---

type Repository[T any] struct {
	data map[string]T
}

func (r *Repository[T]) Get(id string) (T, error) {
	v, ok := r.data[id]
	if !ok {
		return v, fmt.Errorf("not found: %s", id)
	}
	return v, nil
}

func FetchFromRepo[T any](repo *Repository[T], id string) T {
	// @require -nd repo
	// @require len(id) > 0, "id must not be empty"

	result, _ := repo.Get(id) // @must
	return result
}
