package mem

import (
	"context"
	"testing"
)

func TestNewDB_DisabledReturnsUnavailable(t *testing.T) {
	t.Parallel()

	db := NewDB(Options{Enabled: false})
	if db.Enabled() {
		t.Fatal("Enabled()=true, want false when disabled")
	}

	if err := db.EnsureSchema(context.Background()); err == nil {
		t.Fatal("EnsureSchema(disabled)=nil, want ErrHelixUnavailable")
	}
}
