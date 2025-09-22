package resources

// MergeMapStringString merges key-value pairs from the desired map into the existing map.
//
// If a key in the desired map is not present in the existing map, or if its value differs,
// the key-value pair is added or updated in the existing map. The function ensures that
// the existing map is initialized if it is nil.
//
// The entries included in "existing" map that are not included in the "desired" map are preserved.
//
// It returns true if the existing map was modified (i.e., at least one key-value pair was added or updated),
// and false otherwise.
//
// The existing map is passed as a pointer to allow modification in-place.
func MergeMapStringString(existing *map[string]string, desired map[string]string) bool {
	if existing == nil {
		return false
	}
	if *existing == nil {
		*existing = map[string]string{}
	}

	// for each desired key value set, e.g. labels
	// check if it's present in existing. if not add it to existing.
	// e.g. preserving existing labels while adding those that are in the desired set.
	modified := false
	for desiredKey, desiredValue := range desired {
		if existingValue, exists := (*existing)[desiredKey]; !exists || existingValue != desiredValue {
			(*existing)[desiredKey] = desiredValue
			modified = true
		}
	}
	return modified
}

func CopyMap[M ~map[K]V, K comparable, V any](m M) map[K]V {
	if m == nil {
		return nil
	}

	c := make(map[K]V, len(m))
	for k, v := range m {
		c[k] = v
	}

	return c
}
