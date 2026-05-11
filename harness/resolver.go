package harness

// resolver maps declared (pre-resolution) subjects to fully-qualified
// substrate subjects. Both channel subscriptions and action publishes go
// through the same resolver so the symmetry guarantee in
// contracts/subject-resolution.md holds.
//
// The resolver is constructed once at Imp.Run and is immutable thereafter.
type resolver struct {
	prefix            string
	platformMode      bool
	importerAccountPK string
}

// newResolver constructs a resolver. Returns *ErrConfigInvalid if the
// supplied configuration is incomplete: any mode requires a non-empty
// prefix; platform mode additionally requires a non-empty importer
// account public key (FR-033).
func newResolver(prefix string, platformMode bool, importerAccountPK string) (*resolver, error) {
	if prefix == "" {
		return nil, &ErrConfigInvalid{Field: "prefix", Reason: "empty"}
	}
	if platformMode && importerAccountPK == "" {
		return nil, &ErrConfigInvalid{Field: "importer_account_pk", Reason: "empty in platform mode"}
	}
	return &resolver{
		prefix:            prefix,
		platformMode:      platformMode,
		importerAccountPK: importerAccountPK,
	}, nil
}

// resolve returns the fully-qualified subject for a declared subject.
// Wildcards (* and >) are passed through verbatim — the substrate handles
// patterns.
func (r *resolver) resolve(declared string) string {
	if r.platformMode {
		return r.prefix + "." + r.importerAccountPK + "." + declared
	}
	return r.prefix + "." + declared
}

// resolvedPrefix returns the fully-resolved prefix the resolver applies.
// Used as ImpIdentity.SubjectPrefix.
func (r *resolver) resolvedPrefix() string {
	if r.platformMode {
		return r.prefix + "." + r.importerAccountPK
	}
	return r.prefix
}
