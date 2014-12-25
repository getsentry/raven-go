package raven

// testInterface is an example extension Interface.
//
// See http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html for
// more information about interfaces.
type testInterface struct{}

// Class returns a testInterface Class string.
func (t *testInterface) Class() string { return "com.getsentry.raven-go.test-interface" }

// Culprit returns a testInterface Culprit.
func (t *testInterface) Culprit() string { return "codez" }
