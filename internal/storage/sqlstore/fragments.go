package sqlstore

import "strings"

// likeEscape is the escape character every substring filter declares. SQLite
// has no default one and Postgres defaults to a backslash, so naming it makes
// the two engines agree.
const likeEscape = `\`

// containsClause renders a case-insensitive substring test over one column.
//
// Both sides are folded by the engine rather than in Go, so a column and the
// text searched for are compared under the same rules. The engines differ on
// what lower() does beyond ASCII, and every column filtered this way holds a
// GitHub login, repository name, or one of our own fixed identifiers.
func containsClause(column string) string {
	return "lower(" + column + `) LIKE lower(?) ESCAPE '` + likeEscape + `'`
}

// containsAnyClause renders a substring test that matches any of the columns.
// Each column consumes one argument, so callers bind containsArgument once per
// column named here.
func containsAnyClause(columns ...string) string {
	clauses := make([]string, 0, len(columns))
	for _, column := range columns {
		clauses = append(clauses, containsClause(column))
	}

	return "(" + strings.Join(clauses, "\nOR ") + ")"
}

// containsArguments repeats one search term once per column, in the order
// containsAnyClause named them.
func containsArguments(value string, columns int) []any {
	pattern := containsArgument(value)
	arguments := make([]any, 0, columns)
	for range columns {
		arguments = append(arguments, pattern)
	}

	return arguments
}

// containsArgument wraps a search term as a LIKE pattern. A % or _ the user
// typed matches itself rather than acting as a wildcard.
func containsArgument(value string) string {
	escaped := strings.NewReplacer(
		likeEscape, likeEscape+likeEscape,
		"%", likeEscape+"%",
		"_", likeEscape+"_",
	).Replace(value)

	return "%" + escaped + "%"
}

// optionalScopeClause renders a filter that matches rows in one named scope,
// or only the unscoped rows when nothing is named. It consumes two arguments:
// the same optional value twice.
//
// The parameter is cast because an engine that types its placeholders can
// infer nothing from a bare ? tested only for NULL.
func optionalScopeClause(column string) string {
	return "((" + column + " IS NULL AND CAST(? AS TEXT) IS NULL) OR " + column + " = ?)"
}

// caseFold renders an ORDER BY expression that ignores case. It replaces
// SQLite's COLLATE NOCASE, which no other engine has, and an index declared
// over the same expression still serves it.
func caseFold(expression string) string {
	return "lower(" + expression + ")"
}
