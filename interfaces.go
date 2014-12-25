package raven

// Message is a Sentry Interface for reporting formatted messages.
//
// See http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Message
// for more discussion of this interface.
type Message struct {
	// Required
	Message string `json:"message"`

	// Optional
	Params []interface{} `json:"params,omitempty"`
}

// Class reports the Sentry Message Interface class.
func (m *Message) Class() string { return "sentry.interfaces.Message" }

// Template is a Sentry Interface for providing information about formatting templates.
//
// See http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Template
// for more discussion of this interface.
type Template struct {
	// Required
	Filename    string `json:"filename"`
	Lineno      int    `json:"lineno"`
	ContextLine string `json:"context_line"`

	// Optional
	PreContext   []string `json:"pre_context,omitempty"`
	PostContext  []string `json:"post_context,omitempty"`
	AbsolutePath string   `json:"abs_path,omitempty"`
}

// Class reports the Sentry Template Interface class.
func (t *Template) Class() string { return "sentry.interfaces.Template" }

// User is a Sentry Interface for providing information about an affected User.
//
// See http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.User
// for more discussion of this interface.
type User struct {
	Id       string `json:"id"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
}

// Class reports the Sentry User Interface class.
func (h *User) Class() string { return "sentry.interfaces.User" }

// Query is a Sentry Interface for providing information about a database query.
//
// See http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Query
// for more discussion of this interface.
type Query struct {
	// Required
	Query string `json:"query"`

	// Optional
	Engine string `json:"engine,omitempty"`
}

// Class reports the Sentry Query Interface class.
func (q *Query) Class() string { return "sentry.interfaces.Query" }
