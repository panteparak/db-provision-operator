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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock/v4"
)

var _ = Describe("reassignAndDropOwned", func() {
	var (
		ctx  context.Context
		mock pgxmock.PgxPoolIface
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		mock, err = pgxmock.NewPool()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		mock.Close()
	})

	It("issues REASSIGN OWNED then DROP OWNED with the principal escaped", func() {
		mock.ExpectExec(`REASSIGN OWNED BY "alice" TO CURRENT_USER`).
			WillReturnResult(pgxmock.NewResult("REASSIGN", 0))
		mock.ExpectExec(`DROP OWNED BY "alice"`).
			WillReturnResult(pgxmock.NewResult("DROP", 0))

		reassignAndDropOwned(ctx, mock, "alice")

		Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
	})

	It("escapes embedded double-quote characters in the principal", func() {
		mock.ExpectExec(`REASSIGN OWNED BY "weird""name" TO CURRENT_USER`).
			WillReturnResult(pgxmock.NewResult("REASSIGN", 0))
		mock.ExpectExec(`DROP OWNED BY "weird""name"`).
			WillReturnResult(pgxmock.NewResult("DROP", 0))

		reassignAndDropOwned(ctx, mock, `weird"name`)

		Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
	})

	It("ignores errors from REASSIGN OWNED and still issues DROP OWNED", func() {
		mock.ExpectExec(`REASSIGN OWNED BY "ghost" TO CURRENT_USER`).
			WillReturnError(fmt.Errorf("role does not own anything"))
		mock.ExpectExec(`DROP OWNED BY "ghost"`).
			WillReturnResult(pgxmock.NewResult("DROP", 0))

		reassignAndDropOwned(ctx, mock, "ghost")

		Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
	})

	It("ignores errors from DROP OWNED", func() {
		mock.ExpectExec(`REASSIGN OWNED BY "ghost" TO CURRENT_USER`).
			WillReturnResult(pgxmock.NewResult("REASSIGN", 0))
		mock.ExpectExec(`DROP OWNED BY "ghost"`).
			WillReturnError(fmt.Errorf("permission denied"))

		reassignAndDropOwned(ctx, mock, "ghost")

		Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
	})
})
