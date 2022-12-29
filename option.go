package fireql

// Option is the type to replace default parameters.
// fireql.New accepts any number of options (this is functional option pattern).
type Option func(prompt *FireQL) error

// OptionServiceAccount to set a service account to be used for Firestore authentication.
func OptionServiceAccount(serviceAccount string) Option {
	return func(fql *FireQL) error {
		fql.serviceAccount = serviceAccount
		return nil
	}
}

// OptionDefaultLimit to use as the default limit of resulted records.
// Considered only when LIMIT not used in SQL query.
func OptionDefaultLimit(limit int) Option {
	return func(fql *FireQL) error {
		fql.defaultLimit = limit
		return nil
	}
}
