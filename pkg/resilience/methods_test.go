package resilience

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	// Every generated package registers its descriptors here. A new .proto
	// file must be added to this list; the test below fails until it is.
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/analytics/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/expiry/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/feedback/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/freshness/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/notification/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/ranking/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/search/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/stock-sync/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/substitution/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/trust/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/user-profile/v1"
	_ "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/watchlist/v1"
)

// protoRoot is the proto/ directory, relative to this package.
const protoRoot = "../../proto"

// protoMethods returns the full gRPC name of every RPC declared in proto/,
// with its descriptor. It reads the .proto files on disk so a new file cannot
// be missed, and the registered descriptors for the exact names.
func protoMethods(t *testing.T) map[string]protoreflect.MethodDescriptor {
	t.Helper()
	methods := map[string]protoreflect.MethodDescriptor{}
	files := 0
	err := filepath.WalkDir(protoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "gen" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".proto") {
			return nil
		}
		rel, err := filepath.Rel(protoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		files++

		fd, err := protoregistry.GlobalFiles.FindFileByPath(rel)
		if err != nil {
			t.Errorf("%s is not registered: import its generated package in methods_test.go (%v)", rel, err)
			return nil
		}
		for i := 0; i < fd.Services().Len(); i++ {
			svc := fd.Services().Get(i)
			for j := 0; j < svc.Methods().Len(); j++ {
				m := svc.Methods().Get(j)
				methods["/"+string(svc.FullName())+"/"+string(m.Name())] = m
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", protoRoot, err)
	}
	if files == 0 {
		t.Fatalf("no .proto files found under %s", protoRoot)
	}
	return methods
}

// TestEveryProtoRPCHasARetryDecision fails when an RPC is added without being
// put on exactly one of the lists in methods.go.
func TestEveryProtoRPCHasARetryDecision(t *testing.T) {
	methods := protoMethods(t)

	names := make([]string, 0, len(methods))
	for name := range methods {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		_, retryable := retryableMethods[name]
		_, nonRetryable := nonRetryableMethods[name]
		switch {
		case retryable && nonRetryable:
			t.Errorf("%s is in both retryableMethods and nonRetryableMethods", name)
		case !retryable && !nonRetryable:
			t.Errorf("%s has no retry decision: add it to retryableMethods (read-only) "+
				"or nonRetryableMethods (changes state) in pkg/resilience/methods.go", name)
		}

		// A method the proto marks as safe to repeat must be retryable.
		if opts, ok := methods[name].Options().(*descriptorpb.MethodOptions); ok {
			switch opts.GetIdempotencyLevel() {
			case descriptorpb.MethodOptions_NO_SIDE_EFFECTS, descriptorpb.MethodOptions_IDEMPOTENT:
				if !retryable {
					t.Errorf("%s is marked %v in the proto but is not in retryableMethods",
						name, opts.GetIdempotencyLevel())
				}
			}
		}
	}

	// No stale entries: every listed method must still exist.
	for _, list := range []map[string]struct{}{retryableMethods, nonRetryableMethods} {
		for name := range list {
			if _, ok := methods[name]; !ok {
				t.Errorf("%s is listed in methods.go but is not an RPC in proto/", name)
			}
		}
	}
}

func TestDefaultPolicyRetriesOnlyRetryableMethods(t *testing.T) {
	for name := range retryableMethods {
		if !isIdempotentMethod(name) {
			t.Errorf("isIdempotentMethod(%s) = false, want true", name)
		}
	}
	for name := range nonRetryableMethods {
		if isIdempotentMethod(name) {
			t.Errorf("isIdempotentMethod(%s) = true, want false", name)
		}
	}
	if isIdempotentMethod("/swiftmed.new.v1.NewService/DoSomething") {
		t.Error("an unknown method is retryable, want it called once")
	}
}
