package management

import (
	"context"
	"net/url"
	"slices"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/collection"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

var collectionCapabilities = []string{"", "web.http", "web.dns", "web.tls", "web.rdap", "steam.details", "steam.news", "steam.players"}
var executionStates = []string{"", "queued", "running", "retry_wait", "cancel_requested", "succeeded", "partial", "failed", "cancelled"}

func entityFilter(v url.Values) bool {
	return slices.Contains([]string{"", "sites", "games"}, v.Get("kind")) &&
		(v.Get("entity") == "" || catalog.ValidPublicID(v.Get("entity"))) && slices.Contains(collectionCapabilities, v.Get("capability"))
}

func (o *Operations) collectionControl(g fiber.Router) {
	const base = "/operations/collection"
	g.Get(base+"/summary", func(c fiber.Ctx) error {
		summary, err := o.Collector.ControlSummary(c.Context(), dbgen.New(o.Pool))
		if err != nil {
			return failure(err)
		}
		return c.JSON(summary)
	})
	g.Get(base+"/plans", func(c fiber.Ctx) error {
		v, err := search(c, "q", "kind", "entity", "capability", "cursor")
		if err != nil || !entityFilter(v) {
			return fiber.ErrBadRequest
		}
		cursor, err := collectionUUID(v.Get("cursor"))
		if err != nil {
			return err
		}
		out, err := o.Collector.ManagedPlans(c.Context(), dbgen.New(o.Pool), dbgen.ListManagementPlanOwnersParams{
			Kind: v.Get("kind"), PublicID: v.Get("entity"), Capability: v.Get("capability"), Keyword: v.Get("q"), AfterPlan: cursor,
		})
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
	g.Get(base+"/jobs", func(c fiber.Ctx) error {
		v, err := search(c, "q", "kind", "entity", "capability", "cursor", "group", "state", "since", "until", "plan")
		if err != nil || !entityFilter(v) || !slices.Contains([]string{"queue", "history"}, v.Get("group")) ||
			!slices.Contains(executionStates, v.Get("state")) {
			return fiber.ErrBadRequest
		}
		plan, err := collectionUUID(v.Get("plan"))
		if err != nil {
			return fiber.ErrBadRequest
		}
		remote := url.Values{"group": {v.Get("group")}}
		for _, key := range []string{"capability", "state", "since", "until"} {
			if value := v.Get(key); value != "" {
				remote.Set(key, value)
			}
		}
		for _, key := range []string{"since", "until"} {
			if remote.Has(key) {
				if _, err := time.Parse(time.RFC3339Nano, remote.Get(key)); err != nil {
					return fiber.ErrBadRequest
				}
			}
		}
		if cursor, err := collectionUUID(v.Get("cursor")); err != nil {
			return err
		} else if cursor != nil {
			remote.Set("before", cursor.String())
		}
		out, err := o.Collector.ManagedJobs(c.Context(), dbgen.New(o.Pool), collection.JobsFilter{
			Values: remote, Entity: v.Get("entity"), Keyword: v.Get("q"), EntityKind: v.Get("kind"), Plan: plan,
		})
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
	g.Get(base+"/jobs/:job", func(c fiber.Ctx) error {
		job, err := collectionUUID(c.Params("job"))
		v, queryErr := search(c, "entity")
		if err != nil || job == nil || queryErr != nil || !catalog.ValidPublicID(v.Get("entity")) {
			return fiber.ErrBadRequest
		}
		out, err := o.Collector.ManagedJob(c.Context(), dbgen.New(o.Pool), *job, v.Get("entity"))
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
	g.Post(base+"/jobs/:job/:action", editOnly, func(c fiber.Ctx) error {
		job, err := collectionUUID(c.Params("job"))
		action := c.Params("action")
		var in struct {
			Entity string `json:"entity"`
		}
		if err != nil || job == nil || (action != "cancel" && action != "retry") || body(c, &in) != nil || !catalog.ValidPublicID(in.Entity) {
			return fiber.ErrBadRequest
		}
		return o.collectionMutation(c, "collection.job."+action, in.Entity, func(ctx context.Context, q *dbgen.Queries) error {
			return o.Collector.ChangeManagedJob(ctx, q, *job, in.Entity, action)
		})
	})
	for _, kind := range []string{"sites", "games"} {
		o.entityCollectionControls(g, kind)
	}
}

func (o *Operations) collectionMutation(c fiber.Ctx, action, id string, run func(context.Context, *dbgen.Queries) error) error {
	if !catalog.ValidPublicID(id) {
		return fiber.ErrBadRequest
	}
	out, err := o.Catalog.Manage(c.Context(), receipt(c, action), func(ctx context.Context, _ *catalog.Service, q *dbgen.Queries) (catalog.ManagementResult, error) {
		return catalog.ManagementResult{Target: id}, run(ctx, q)
	})
	if err != nil {
		return failure(err)
	}
	return c.JSON(out)
}

func (o *Operations) entityCollectionControls(g fiber.Router, kind string) {
	base := "/" + kind + "/:id/collection"
	g.Post(base+"/register", editOnly, func(c fiber.Ctx) error {
		if o.Collector == nil {
			return fiber.ErrServiceUnavailable
		}
		var in struct {
			AppID string `json:"app_id,omitempty"`
		}
		if err := body(c, &in); err != nil {
			return err
		}
		id := c.Params("id")
		return o.collectionMutation(c, "collection.register", id, func(ctx context.Context, q *dbgen.Queries) error {
			if kind == "sites" && in.AppID == "" {
				return o.Collector.RegisterSite(ctx, q, id, o.Region)
			}
			if kind == "games" {
				return o.Collector.RegisterSteam(ctx, q, id, in.AppID, o.Region)
			}
			return collection.ErrIntent
		})
	})
	g.Post(base+"/plans", editOnly, func(c fiber.Ctx) error {
		var in collection.AddPlanInput
		if err := body(c, &in); err != nil {
			return err
		}
		id := c.Params("id")
		return o.collectionMutation(c, "collection.plan.create", id, func(ctx context.Context, q *dbgen.Queries) error {
			return o.Collector.CreateManagedPlan(ctx, q, kind, id, o.Region, in)
		})
	})
	g.Get(base+"/:plan/results", func(c fiber.Ctx) error {
		id := c.Params("id")
		plan, err := collectionUUID(c.Params("plan"))
		if err != nil || plan == nil || !catalog.ValidPublicID(id) {
			return fiber.ErrBadRequest
		}
		out, err := o.Collector.PlanResults(c.Context(), dbgen.New(o.Pool), kind, id, *plan)
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
	for _, action := range []string{"state", "run"} {
		g.Post(base+"/:plan/"+action, editOnly, func(c fiber.Ctx) error {
			id := c.Params("id")
			plan, err := collectionUUID(c.Params("plan"))
			if err != nil || plan == nil {
				return fiber.ErrBadRequest
			}
			var input collection.PlanControlInput
			if err := body(c, &input); err != nil {
				return err
			}
			return o.collectionMutation(c, "collection.plan."+action, id, func(ctx context.Context, q *dbgen.Queries) error {
				if action == "state" {
					return o.Collector.SetPlanEnabled(ctx, q, kind, id, *plan, input)
				}
				if input.Enabled != nil || input.Expected != nil {
					return collection.ErrIntent
				}
				return o.Collector.RunManagedPlan(ctx, q, kind, id, *plan)
			})
		})
	}
}
