package resources

import (
	"reflect"
	"testing"
)

func TestMergeMapStringString(t *testing.T) {
	m := make(map[string]string)
	var nilMap map[string]string

	type args struct {
		existing *map[string]string
		desired  map[string]string
	}
	tests := []struct {
		name       string
		args       args
		wantUpdate bool
	}{
		{
			name: "nil pointer to map",
			args: args{
				existing: nil,
				desired: map[string]string{
					"foo": "bar",
					"qux": "quux",
				},
			},
			wantUpdate: false,
		},
		{
			name: "nil map",
			args: args{
				existing: &nilMap,
				desired: map[string]string{
					"foo": "bar",
					"qux": "quux",
				},
			},
			wantUpdate: true,
		},
		{
			name: "empty map",
			args: args{
				existing: &m,
				desired: map[string]string{
					"foo": "bar",
					"qux": "quux",
				},
			},
			wantUpdate: true,
		},
		{
			name: "desired keys not in existing",
			args: args{
				existing: &map[string]string{
					"foo": "bar",
				},
				desired: map[string]string{
					"qux": "quux",
				},
			},
			wantUpdate: true,
		},
		{
			name: "same maps",
			args: args{
				existing: &map[string]string{
					"foo": "bar",
					"qux": "quux",
				},
				desired: map[string]string{
					"foo": "bar",
					"qux": "quux",
				},
			},
			wantUpdate: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			got := MergeMapStringString(tt.args.existing, tt.args.desired)
			if got != tt.wantUpdate {
				subT.Errorf("MergeMapStringString() got = %v, wantUpdate %v", got, tt.wantUpdate)
			}

			if tt.args.existing == nil {
				return
			}

			for desiredKey, desiredValue := range tt.args.desired {
				existingVal, ok := (*tt.args.existing)[desiredKey]
				if !ok || existingVal != desiredValue {
					t.Errorf("MergeMapStringString() the key does not match:  %v", desiredKey)
				}
			}
		})
	}
}

func TestCopyMap(t *testing.T) {
	// Test Case 1: A standard, non-empty map
	t.Run("copy non-empty map", func(t *testing.T) {
		original := map[string]string{
			"hello": "world",
			"go":    "lang",
		}
		copied := CopyMap(original)

		// 1. Check if the copy is deeply equal to the original.
		if !reflect.DeepEqual(original, copied) {
			t.Errorf("Copied map is not equal to the original. Got %v, want %v", copied, original)
		}

		// 2. Check for independence: modifying the original should not affect the copy.
		original["new_key"] = "new_value"
		if _, exists := copied["new_key"]; exists {
			t.Error("Copied map was modified when the original was changed.")
		}
	})

	// Test Case 2: An empty map
	t.Run("copy empty map", func(t *testing.T) {
		original := map[string]string{}
		copied := CopyMap(original)

		if copied == nil {
			t.Error("Copying an empty map should not result in a nil map.")
		}
		if len(copied) != 0 {
			t.Errorf("Copied map of an empty map should be empty. Got length %d", len(copied))
		}
	})

	// Test Case 3: A nil map
	t.Run("copy nil map", func(t *testing.T) {
		var original map[string]string = nil
		copied := CopyMap(original)

		if copied != nil {
			t.Errorf("Copying a nil map should result in a nil map. Got %v", copied)
		}
	})
}
