package wire

import "errors"

// Wire failure classes and user guidance.

// Sentinel errors let callers branch on wire failure classes. Every failure
// wires returns wraps one of these with %w and operation context.
var (
	// ErrBadURL is an unparseable or unsupported source URL.
	ErrBadURL = errors.New("wire: unsupported repository URL")
	// ErrUnknownRef names a reference absent on the remote.
	ErrUnknownRef = errors.New("wire: unknown reference")
	// ErrMissingPath names a subpath absent under a known reference.
	ErrMissingPath = errors.New("wire: subfolder not found upstream")
	// ErrTargetNotEmpty guards a non-empty fetch target without --force.
	ErrTargetNotEmpty = errors.New("wire: target directory not empty")
	// ErrNotACheckout marks a directory without a tracking record.
	ErrNotACheckout = errors.New("wire: not a git-wire checkout")
	// ErrInvalidRecord marks a corrupt or unsupported tracking record.
	ErrInvalidRecord = errors.New("wire: invalid tracking record")
	// ErrDiverged guards locally modified checkouts on update without --force.
	ErrDiverged = errors.New("wire: local modifications present")
	// ErrUpstream marks network, auth, and rate-limit failures.
	ErrUpstream = errors.New("wire: upstream unreachable")
)

// NextAction returns the user-facing next step for a wire failure class.
// It never returns empty; unknown errors yield generic guidance.
func NextAction(err error) string {
	switch {
	case errors.Is(err, ErrBadURL):
		return "expected shape: https://{host}/{owner}/{repo}/tree/{ref}/{path}"
	case errors.Is(err, ErrUnknownRef):
		return "check the branch, tag, or commit spelling and try again"
	case errors.Is(err, ErrMissingPath):
		return "check the subfolder path for the given reference"
	case errors.Is(err, ErrTargetNotEmpty):
		return "use --force to replace the target, or choose an empty directory"
	case errors.Is(err, ErrNotACheckout):
		return "fetch the folder first with its source URL"
	case errors.Is(err, ErrInvalidRecord):
		return "re-fetch the folder to regenerate its tracking record"
	case errors.Is(err, ErrDiverged):
		return "back up local edits, then use --force to discard them"
	case errors.Is(err, ErrUpstream):
		return "retry later; local files were left intact"
	default:
		return "retry the command; local files were left intact"
	}
}
