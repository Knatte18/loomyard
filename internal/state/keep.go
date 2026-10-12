// keep.go merges the keys of the JSON file a write replaces into the write's own encoding, so a record a newer binary wrote keeps the keys this binary's Go type does not declare.

package state

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
)

var (
	marshalerType   = reflect.TypeFor[json.Marshaler]()
	unmarshalerType = reflect.TypeFor[json.Unmarshaler]()
	rawMessageType  = reflect.TypeFor[json.RawMessage]()
	nullJSON        = []byte("null")
)

// jsonMember is one member of a JSON object: its decoded key, the key's text as it stands in the source, and the raw bytes of its value.
type jsonMember struct {
	key    string
	rawKey []byte
	value  []byte
}

// keepUnknown returns newRaw with every object member of oldRaw that the Go type t does not declare appended to the object that holds it, recursing through t's struct, pointer, map, slice and array layers.
// Both inputs are compact or indented JSON; the result is compact wherever it differs from newRaw.
// A type that encodes itself (a json.Marshaler or json.Unmarshaler, json.RawMessage, an interface or a scalar kind) is opaque and yields newRaw whole, as does any pair whose JSON shapes differ.
// A slice or array element is merged with an old element only when both decode to the same value, so an element updated in place keeps no unknown keys, while one that merely shifted index keeps them.
func keepUnknown(t reflect.Type, oldRaw, newRaw []byte) []byte {
	if isOpaque(t) {
		return newRaw
	}
	oldRaw = bytes.TrimSpace(oldRaw)
	newRaw = bytes.TrimSpace(newRaw)
	switch t.Kind() {
	case reflect.Pointer:
		if bytes.Equal(oldRaw, nullJSON) || bytes.Equal(newRaw, nullJSON) {
			return newRaw
		}
		return keepUnknown(t.Elem(), oldRaw, newRaw)
	case reflect.Struct:
		return keepUnknownInStruct(t, oldRaw, newRaw)
	case reflect.Map:
		return keepUnknownInMap(t, oldRaw, newRaw)
	case reflect.Slice, reflect.Array:
		return keepUnknownInList(t, oldRaw, newRaw)
	}
	return newRaw
}

// isOpaque reports whether t encodes itself or has no object or array shape to merge into.
func isOpaque(t reflect.Type) bool {
	if t == rawMessageType {
		return true
	}
	for _, codec := range []reflect.Type{marshalerType, unmarshalerType} {
		if t.Implements(codec) || reflect.PointerTo(t).Implements(codec) {
			return true
		}
	}
	switch t.Kind() {
	case reflect.Struct, reflect.Pointer, reflect.Map, reflect.Slice, reflect.Array:
		return false
	}
	return true
}

// keepUnknownInStruct merges oldRaw into newRaw for a struct type: the new object's members in their order, each recursed into when the old object holds the same key, then the old object's undeclared members in the old order.
func keepUnknownInStruct(t reflect.Type, oldRaw, newRaw []byte) []byte {
	oldMembers, oldIsObject := objectMembers(oldRaw)
	newMembers, newIsObject := objectMembers(newRaw)
	if !oldIsObject || !newIsObject {
		return newRaw
	}
	declared := declaredFields(t)
	oldValues := lastValueByKey(oldMembers)

	merged := make([]jsonMember, 0, len(newMembers))
	written := make(map[string]bool, len(newMembers))
	for _, member := range newMembers {
		written[member.key] = true
		if oldValue, held := oldValues[member.key]; held {
			if fieldType, known := declared.lookup(member.key); known {
				member.value = keepUnknown(fieldType, oldValue, member.value)
			}
		}
		merged = append(merged, member)
	}
	for _, member := range oldMembers {
		if written[member.key] {
			continue
		}
		if _, known := declared.lookup(member.key); known {
			continue
		}
		written[member.key] = true
		member.value = oldValues[member.key]
		merged = append(merged, member)
	}
	return encodeObject(merged)
}

// keepUnknownInMap merges oldRaw into newRaw for a map type: each value under a key both objects hold is recursed into, and no key the new map lacks is added.
func keepUnknownInMap(t reflect.Type, oldRaw, newRaw []byte) []byte {
	oldMembers, oldIsObject := objectMembers(oldRaw)
	newMembers, newIsObject := objectMembers(newRaw)
	if !oldIsObject || !newIsObject {
		return newRaw
	}
	oldValues := lastValueByKey(oldMembers)
	for i, member := range newMembers {
		if oldValue, held := oldValues[member.key]; held {
			newMembers[i].value = keepUnknown(t.Elem(), oldValue, member.value)
		}
	}
	return encodeObject(newMembers)
}

// keepUnknownInList merges oldRaw into newRaw for a slice or array type, element by element, for each element that has a not yet paired old element encoding to the same compact JSON, the one at the same index preferred.
func keepUnknownInList(t reflect.Type, oldRaw, newRaw []byte) []byte {
	if isOpaque(t.Elem()) {
		return newRaw
	}
	oldElements, oldIsArray := arrayElements(oldRaw)
	newElements, newIsArray := arrayElements(newRaw)
	if !oldIsArray || !newIsArray {
		return newRaw
	}
	paired := make([]bool, len(oldElements))
	for i, element := range newElements {
		if oldIndex, found := pairOldElement(t.Elem(), oldElements, paired, i, element); found {
			paired[oldIndex] = true
			newElements[i] = keepUnknown(t.Elem(), oldElements[oldIndex], element)
		}
	}
	return encodeArray(newElements)
}

// pairOldElement returns the index of the old element not yet paired that encodes as newElement, preferring preferred and otherwise the first such element.
func pairOldElement(t reflect.Type, oldElements [][]byte, paired []bool, preferred int, newElement []byte) (int, bool) {
	if preferred < len(oldElements) && !paired[preferred] && encodesAs(t, oldElements[preferred], newElement) {
		return preferred, true
	}
	for i, oldElement := range oldElements {
		if !paired[i] && encodesAs(t, oldElement, newElement) {
			return i, true
		}
	}
	return 0, false
}

// encodesAs reports whether oldRaw, decoded into a value of type t and encoded compact again, is byte for byte newRaw.
func encodesAs(t reflect.Type, oldRaw, newRaw []byte) bool {
	decoded := reflect.New(t)
	if err := json.Unmarshal(oldRaw, decoded.Interface()); err != nil {
		return false
	}
	reencoded, err := json.Marshal(decoded.Elem().Interface())
	if err != nil {
		return false
	}
	return bytes.Equal(reencoded, newRaw)
}

// fieldSet is the JSON keys a struct type declares, with the Go type each decodes into.
type fieldSet struct {
	exact map[string]reflect.Type
	// names lists the keys in declaration order, for the case-insensitive fallback encoding/json applies when no key matches exactly.
	names []string
}

// declaredFields returns the keys t declares: json tag names and field names, embedded structs' promoted fields included and `json:"-"` fields excluded.
func declaredFields(t reflect.Type) fieldSet {
	declared := fieldSet{exact: map[string]reflect.Type{}}
	for _, field := range reflect.VisibleFields(t) {
		tag, tagged := field.Tag.Lookup("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if field.Anonymous && name == "" {
			// An untagged embedded struct contributes its promoted fields, listed after it.
			embedded := field.Type
			if embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				continue
			}
		}
		if !field.IsExported() {
			continue
		}
		if !tagged || name == "" {
			name = field.Name
		}
		if _, seen := declared.exact[name]; !seen {
			declared.names = append(declared.names, name)
			declared.exact[name] = field.Type
		}
	}
	return declared
}

// lookup returns the type of the field key decodes into: the field whose name equals key, else the first whose name equals it ignoring case, as encoding/json matches.
func (declared fieldSet) lookup(key string) (reflect.Type, bool) {
	if fieldType, found := declared.exact[key]; found {
		return fieldType, true
	}
	for _, name := range declared.names {
		if strings.EqualFold(name, key) {
			return declared.exact[name], true
		}
	}
	return nil, false
}

// lastValueByKey maps each key to the value of its last member, the one encoding/json decodes.
func lastValueByKey(members []jsonMember) map[string][]byte {
	values := make(map[string][]byte, len(members))
	for _, member := range members {
		values[member.key] = member.value
	}
	return values
}

// objectMembers splits a JSON object into its members in source order, and reports false when raw is not exactly one object.
func objectMembers(raw []byte) ([]jsonMember, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, false
	}
	var members []jsonMember
	for decoder.More() {
		start := decoder.InputOffset()
		token, err := decoder.Token()
		key, isString := token.(string)
		if err != nil || !isString {
			return nil, false
		}
		rawKey := bytes.TrimLeft(raw[start:decoder.InputOffset()], ", \t\r\n")
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, false
		}
		members = append(members, jsonMember{key: key, rawKey: rawKey, value: value})
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, false
	}
	return members, decoder.InputOffset() == int64(len(raw))
}

// arrayElements splits a JSON array into its raw elements, and reports false when raw is not exactly one array.
func arrayElements(raw []byte) ([][]byte, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
		return nil, false
	}
	var elements [][]byte
	for decoder.More() {
		var element json.RawMessage
		if err := decoder.Decode(&element); err != nil {
			return nil, false
		}
		elements = append(elements, element)
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
		return nil, false
	}
	return elements, decoder.InputOffset() == int64(len(raw))
}

// encodeObject joins members into one compact JSON object, keeping each key's source text.
func encodeObject(members []jsonMember) []byte {
	var out bytes.Buffer
	out.WriteByte('{')
	for i, member := range members {
		if i > 0 {
			out.WriteByte(',')
		}
		out.Write(member.rawKey)
		out.WriteByte(':')
		out.Write(member.value)
	}
	out.WriteByte('}')
	return out.Bytes()
}

// encodeArray joins elements into one compact JSON array.
func encodeArray(elements [][]byte) []byte {
	return append(append([]byte{'['}, bytes.Join(elements, []byte{','})...), ']')
}
