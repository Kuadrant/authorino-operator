package resources

import (
	"slices"
	"testing"

	k8srbac "k8s.io/api/rbac/v1"
)

func TestMergeBindingSubject(t *testing.T) {
	subjectFoo := k8srbac.Subject{Kind: "Foo"}
	subjectBar := k8srbac.Subject{Kind: "Bar"}

	emptySlice := make([]k8srbac.Subject, 0)
	var nilSlice []k8srbac.Subject

	type args struct {
		existing *[]k8srbac.Subject
		desired  []k8srbac.Subject
	}
	tests := []struct {
		name       string
		args       args
		wantUpdate bool
	}{
		{
			name: "nil pointer to slice",
			args: args{
				existing: nil,
				desired:  []k8srbac.Subject{subjectFoo, subjectBar},
			},
			wantUpdate: false,
		},
		{
			name: "nil slice",
			args: args{
				existing: &nilSlice,
				desired:  []k8srbac.Subject{subjectFoo, subjectBar},
			},
			wantUpdate: true,
		},
		{
			name: "empty slice",
			args: args{
				existing: &emptySlice,
				desired:  []k8srbac.Subject{subjectFoo, subjectBar},
			},
			wantUpdate: true,
		},
		{
			name: "desired subjects not in existing",
			args: args{
				existing: &[]k8srbac.Subject{subjectFoo},
				desired:  []k8srbac.Subject{subjectBar},
			},
			wantUpdate: true,
		},
		{
			name: "same slices",
			args: args{
				existing: &[]k8srbac.Subject{subjectFoo, subjectBar},
				desired:  []k8srbac.Subject{subjectFoo, subjectBar},
			},
			wantUpdate: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			got := MergeBindingSubject(tt.args.desired, tt.args.existing)
			if got != tt.wantUpdate {
				subT.Errorf("MergeBindingSubject() got = %v, wantUpdate %v", got, tt.wantUpdate)
			}

			if tt.args.existing == nil {
				return
			}

			if len(*tt.args.existing) < len(tt.args.desired) {
				subT.Error("existing has less subjects than desired")
			}

			for idx := range tt.args.desired {
				if !slices.Contains(*tt.args.existing, tt.args.desired[idx]) {
					t.Errorf("MergeBindingSubject() desired subject not in existing: %v", tt.args.desired[idx])
				}
			}
		})
	}
}
