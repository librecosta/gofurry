package management

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/collection"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func (o *Operations) collectionWorkers(g fiber.Router) {
	const base = "/operations/collection/worker-nodes"
	canOperate := func(c fiber.Ctx) bool {
		role := Current(c).Role
		return role == "owner" || role == "operator"
	}
	g.Get(base, func(c fiber.Ctx) error {
		v, err := search(c, "cursor")
		if err != nil {
			return err
		}
		out, err := o.Collector.ManagedWorkerNodes(c.Context(), canOperate(c), v.Get("cursor"))
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
	g.Get(base+"/:node", func(c fiber.Ctx) error {
		if _, err := search(c); err != nil {
			return err
		}
		out, err := o.Collector.ManagedWorkerNode(c.Context(), canOperate(c), c.Params("node"))
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
	g.Post(base+"/:node/state", editOnly, func(c fiber.Ctx) error {
		if _, err := search(c); err != nil {
			return err
		}
		var input collection.WorkerNodeEdit
		if err := body(c, &input); err != nil {
			return err
		}
		id := c.Params("node")
		actor := receipt(c, "collection.worker.state")
		out, err := o.Catalog.Manage(c.Context(), actor, func(ctx context.Context, _ *catalog.Service, _ *dbgen.Queries) (catalog.ManagementResult, error) {
			revision, err := o.Collector.ChangeWorkerNode(ctx, id, input, actor)
			return catalog.ManagementResult{Target: id, Revision: revision}, err
		})
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
}
