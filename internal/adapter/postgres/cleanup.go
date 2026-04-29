/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// poolExecer is the minimal pool capability needed for cleanup statements.
// Both *pgxpool.Pool and the pgxmock test fakes satisfy it.
type poolExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// reassignAndDropOwned reassigns objects owned by principal to CURRENT_USER and
// drops any remaining owned privileges. Errors from both statements are
// intentionally swallowed: this is best-effort cleanup ahead of DROP ROLE so
// the deletion stays idempotent (e.g. the principal owns nothing, or has been
// partially cleaned up by an earlier failed run).
func reassignAndDropOwned(ctx context.Context, pool poolExecer, principal string) {
	quoted := escapeIdentifier(principal)
	_, _ = pool.Exec(ctx, fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", quoted))
	_, _ = pool.Exec(ctx, fmt.Sprintf("DROP OWNED BY %s", quoted))
}
