package scanner

import (
	"database/sql"
	"reflect"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// binder populates ptrs[colIdx] with an interface pointing to the target field in basePtr.
type binder func(basePtr unsafe.Pointer, ptrs []any, colIdx int)

type fieldMeta struct {
	offset uintptr
	bindFn binder
}

type meta struct {
	fields map[string]fieldMeta
}

type discardScanner struct{}

func (discardScanner) Scan(any) error {
	return nil
}

var (
	cache   sync.Map
	discard discardScanner
	ptrPool = sync.Pool{
		New: func() any {
			s := make([]any, 32)
			return &s
		},
	}
)

// MapRaw maps sql.Rows to a slice of struct T.
func MapRaw[T any](rows *sql.Rows) ([]T, error) {
	defer rows.Close()

	results := make([]T, 0, 16)

	t := reflect.TypeFor[T]()
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	mObj, ok := cache.Load(t)
	if !ok {
		mObj, _ = cache.LoadOrStore(t, parseMeta(t))
	}
	m := mObj.(*meta)

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	colCount := len(cols)

	// Pre-extract binders once per query
	binders := make([]binder, colCount)
	for i, name := range cols {
		if fMeta, exists := m.fields[name]; exists {
			binders[i] = fMeta.bindFn
		}
	}

	ptrToSlice := ptrPool.Get().(*[]any)
	ptrs := *ptrToSlice
	if cap(ptrs) < colCount {
		ptrs = make([]any, colCount)
	} else {
		ptrs = ptrs[:colCount]
	}

	for rows.Next() {
		results = append(results, *new(T))
		itemPtr := unsafe.Pointer(&results[len(results)-1])

		// Execute pre-compiled binding: zero reflect calls inside the loop!
		for i, bind := range binders {
			if bind != nil {
				bind(itemPtr, ptrs, i)
			} else {
				ptrs[i] = &discard
			}
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
	}

	// Reset slice to avoid retaining pointers
	for i := range ptrs {
		ptrs[i] = nil
	}
	*ptrToSlice = ptrs[:0]
	ptrPool.Put(ptrToSlice)

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func parseMeta(t reflect.Type) *meta {
	m := &meta{fields: make(map[string]fieldMeta)}
	extractFields(t, 0, m)
	return m
}

func extractFields(t reflect.Type, baseOffset uintptr, m *meta) {
	if t.Kind() != reflect.Struct {
		return
	}

	for field := range t.Fields() {
		field := field

		// Handle embedded unexported or exported structs recursively
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			extractFields(field.Type, baseOffset+field.Offset, m)
			continue
		}

		tag := field.Tag.Get("db")
		if tag == "-" {
			continue // Explicitly ignored field
		}
		if tag == "" {
			tag = field.Name
		}

		fieldOffset := baseOffset + field.Offset
		fieldType := field.Type

		fMeta := fieldMeta{
			offset: fieldOffset,
			bindFn: makeBinder(fieldType, fieldOffset),
		}

		m.fields[tag] = fMeta
		if lower := strings.ToLower(tag); lower != tag {
			m.fields[lower] = fMeta
		}
	}
}

// makeBinder creates a direct pointer-conversion function for zero-reflection in rows.Next().
func makeBinder(t reflect.Type, offset uintptr) binder {
	// Specialized fast-paths for standard SQL primitive types (avoids reflect.NewAt entirely)
	switch t.Kind() {
	case reflect.Int:
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (*int)(unsafe.Add(base, offset))
		}
	case reflect.Int64:
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (*int64)(unsafe.Add(base, offset))
		}
	case reflect.Int32:
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (*int32)(unsafe.Add(base, offset))
		}
	case reflect.Uint:
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (*uint)(unsafe.Add(base, offset))
		}
	case reflect.Uint64:
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (*uint64)(unsafe.Add(base, offset))
		}
	case reflect.Uint32:
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (*uint32)(unsafe.Add(base, offset))
		}
	case reflect.String:
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (*string)(unsafe.Add(base, offset))
		}
	case reflect.Bool:
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (*bool)(unsafe.Add(base, offset))
		}
	case reflect.Float64:
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (*float64)(unsafe.Add(base, offset))
		}
	case reflect.Float32:
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (*float32)(unsafe.Add(base, offset))
		}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 { // []byte (JSONB, BLOB, TEXT)
			return func(base unsafe.Pointer, ptrs []any, idx int) {
				ptrs[idx] = (*[]byte)(unsafe.Add(base, offset))
			}
		}
	}

	// Fast-path for time.Time without reflection
	if t == reflect.TypeFor[time.Time]() {
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (*time.Time)(unsafe.Add(base, offset))
		}
	}
	if t == reflect.TypeFor[*time.Time]() {
		return func(base unsafe.Pointer, ptrs []any, idx int) {
			ptrs[idx] = (**time.Time)(unsafe.Add(base, offset))
		}
	}

	// Generic fallback for custom types (sql.Null*, custom Scanner types)
	return func(base unsafe.Pointer, ptrs []any, idx int) {
		fieldAddr := unsafe.Add(base, offset)
		ptrs[idx] = reflect.NewAt(t, fieldAddr).Interface()
	}
}
