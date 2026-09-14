package sqlstore

import (
	"context"
	"fmt"
)

// Resolve the full authorized facet set, independently of pagination and search.
// LEFT JOIN preserves identities whose catalogue record is no longer available.
func (s *Store) queueFacetNames(ctx context.Context, targetID *string, profiles bool) (map[string]string, error) {
	query := `SELECT DISTINCT q.repository_id, COALESCE(r.full_name, '')
 FROM queue_items q LEFT JOIN repositories r ON r.id = q.repository_id AND r.target_id = q.target_id
 WHERE q.repository_id IS NOT NULL`
	if profiles {
		query = `SELECT DISTINCT COALESCE(q.profile_id, 'immediate'),
 CASE WHEN q.profile_id IS NULL THEN 'Immediate' ELSE COALESCE(p.name, '') END
 FROM queue_items q LEFT JOIN schedule_profiles p ON p.id = q.profile_id WHERE 1 = 1`
	}
	var args []any
	if targetID != nil {
		query += " AND q.target_id = ?"
		args = append(args, *targetID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query queue facet names: %w", err)
	}
	defer func() { _ = rows.Close() }()
	names := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("read queue facet name: %w", err)
		}
		names[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queue facet names: %w", err)
	}
	return names, nil
}
