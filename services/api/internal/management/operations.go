package management

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"time"
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/collection"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/storage"
)

type Operations struct {
	Pool              *pgxpool.Pool
	Catalog           *catalog.Service
	Assets            *storage.Store
	Intake            *collection.Intake
	Discovery         *collection.Discovery
	Collector         *collection.Service
	Namespace, Region string
	Egress            string
	ReconcileEnabled  bool
	assetSlots        chan struct{}
}

func editOnly(c fiber.Ctx) error {
	if Current(c).Role == "developer" {
		return fiber.ErrForbidden
	}
	return c.Next()
}

func ownerOnly(c fiber.Ctx) error {
	if Current(c).Role != "owner" {
		return fiber.ErrForbidden
	}
	return c.Next()
}

func receipt(c fiber.Ctx, action string) dbgen.RecordManagementOperationParams {
	actor := Current(c)
	id, _ := uuid.Parse(actor.RequestID)
	return dbgen.RecordManagementOperationParams{RequestID: id, ActorID: *actor.ID, ActorName: actor.Name, ActorRole: actor.Role, Action: action}
}

func failure(err error) error {
	var pe *pgconn.PgError
	var duplicate catalog.SiteAddressConflict
	var steamDuplicate collection.SteamGameExists
	switch {
	case err == nil:
		return nil
	case errors.Is(err, catalog.ErrGamePublicationIdentity):
		return fiber.ErrForbidden
	case errors.Is(err, collection.ErrCollectionPermission):
		return fiber.ErrForbidden
	case errors.Is(err, collection.ErrEgressPolicyInvalid):
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, collection.ErrEgressPolicyConflict):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case collection.IsPlanRejection(err):
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, collection.ErrPlanChanged), errors.Is(err, collection.ErrSharedPlan), errors.Is(err, collection.ErrPlanSync):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.Is(err, catalog.ErrGamePublicationText), errors.Is(err, catalog.ErrGamePublicationReview), errors.Is(err, catalog.ErrGamePublicationPolicy):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, catalog.ErrGamePreviewChanged):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.Is(err, catalog.ErrEditorialExists), errors.Is(err, catalog.ErrEditorialCategoryArchived),
		errors.Is(err, catalog.ErrEditorialCategoryInUse), errors.Is(err, catalog.ErrEditorialTagInUse):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.As(err, &duplicate):
		return fiber.NewError(fiber.StatusConflict, duplicate.Error())
	case errors.As(err, &steamDuplicate):
		return fiber.NewError(fiber.StatusConflict, steamDuplicate.Error())
	case errors.Is(err, catalog.ErrSiteNameRequired), errors.Is(err, catalog.ErrSiteChineseRequired),
		errors.Is(err, catalog.ErrSiteAddressRequired), errors.Is(err, catalog.ErrSiteAddressInvalid),
		errors.Is(err, catalog.ErrSiteCategoryRequired), errors.Is(err, catalog.ErrSiteSecondaryConflict), errors.Is(err, catalog.ErrSiteReviewRequired):
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, catalog.ErrInvalid), errors.Is(err, collection.ErrIntakeInput), errors.Is(err, collection.ErrIntent):
		return fiber.ErrBadRequest
	case catalog.ManagementMissing(err):
		return fiber.ErrNotFound
	case catalog.ManagementConflict(err), errors.Is(err, collection.ErrIntakeState), collection.ManagementBusy(err), errors.As(err, &pe) && pe.Code == "23505":
		return fiber.ErrConflict
	case errors.Is(err, collection.ErrIntakeCapacity):
		return fiber.ErrTooManyRequests
	default:
		return fiber.ErrServiceUnavailable
	}
}

func writeOperation[T any](o *Operations, action string, fn func(context.Context, *catalog.Service, *dbgen.Queries, string, T, catalog.PublicationDecision) (catalog.ManagementResult, error)) fiber.Handler {
	return func(c fiber.Ctx) error {
		var input T
		limit := 16384
		if action == "game.rich_details" {
			limit = 256 << 10
		}
		if err := boundedJSON(c, &input, limit); err != nil {
			return err
		}
		id := c.Params("id")
		if id != "" && !catalog.ValidPublicID(id) {
			return fiber.ErrBadRequest
		}
		if code := c.Params("code"); code != "" {
			if !catalog.ValidEditorialCode(code) {
				return fiber.ErrBadRequest
			}
			id = code
		}
		actor := Current(c)
		result, err := o.Catalog.Manage(c.Context(), receipt(c, action), func(ctx context.Context, s *catalog.Service, q *dbgen.Queries) (catalog.ManagementResult, error) {
			return fn(ctx, s, q, id, input, catalog.PublicationDecision{Actor: actor.ID.String()})
		})
		if err != nil {
			return failure(err)
		}
		return c.JSON(result)
	}
}

func readOperation[T any](fn func(context.Context, string) (T, error)) fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Params("id")
		if !catalog.ValidPublicID(id) {
			return fiber.ErrBadRequest
		}
		result, err := fn(c.Context(), id)
		if err != nil {
			return failure(err)
		}
		return c.JSON(result)
	}
}

func search(c fiber.Ctx, allowed ...string) (url.Values, error) {
	raw := string(c.Request().URI().QueryString())
	if len(raw) > 1024 {
		return nil, fiber.ErrBadRequest
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, fiber.ErrBadRequest
	}
	for k, v := range values {
		if !slices.Contains(allowed, k) || len(v) != 1 || len(v[0]) > 400 {
			return nil, fiber.ErrBadRequest
		}
	}
	if !catalog.ManagementSearch(values.Get("q")) {
		return nil, fiber.ErrBadRequest
	}
	if after := values.Get("after"); after != "" && !catalog.ValidPublicID(after) {
		return nil, fiber.ErrBadRequest
	}
	if state := values.Get("status"); state != "" && !slices.Contains([]string{"draft", "published", "archived"}, state) {
		return nil, fiber.ErrBadRequest
	}
	return values, nil
}

const pageSize = 30

func page[T any](c fiber.Ctx, rows []T, err error, limit int, id func(T) string) error {
	if err != nil {
		return failure(err)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = id(rows[len(rows)-1])
	}
	return c.JSON(fiber.Map{"items": rows, "next": next})
}

func (o *Operations) ready(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ready, err := dbgen.New(o.Pool).ManagementSchemaReady(ctx)
	return err == nil && ready
}

func (o *Operations) register(app *fiber.App) {
	o.assetSlots = make(chan struct{}, 2)
	g := app.Group("/internal/v1/ops")
	g.Get("/overview", o.overview)
	g.Get("/receipts/:request", func(c fiber.Ctx) error {
		raw := c.Params("request")
		id, err := uuid.Parse(raw)
		if err != nil || id.String() != raw || id == uuid.Nil() {
			return fiber.ErrBadRequest
		}
		row, err := dbgen.New(o.Pool).ManagementReceipt(c.Context(), dbgen.ManagementReceiptParams{RequestID: id, ActorID: *Current(c).ID})
		if err != nil {
			return failure(err)
		}
		return c.JSON(row)
	})
	o.sites(g)
	o.games(g)
	o.entityCollection(g, "sites")
	o.entityCollection(g, "games")
	o.collectionControl(g)
	o.acquisition(g)
}

type health struct {
	collection.ConsumerHealth
	Cursor       *dbgen.ManagementCollectionStateRow  `json:"cursor"`
	Plans        []dbgen.ManagementCollectionPlansRow `json:"plans"`
	CanReconcile bool                                 `json:"can_reconcile"`
}

func (o *Operations) health(ctx context.Context) (health, error) {
	view := health{ConsumerHealth: o.Collector.ConsumerHealth(), CanReconcile: o.ReconcileEnabled, Plans: []dbgen.ManagementCollectionPlansRow{}}
	if o.Namespace == "" {
		return view, nil
	}
	q := dbgen.New(o.Pool)
	row, err := q.ManagementCollectionState(ctx, o.Namespace)
	if err == nil {
		view.Cursor = &row
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return view, err
	}
	view.Plans, err = q.ManagementCollectionPlans(ctx, o.Namespace)
	return view, err
}

func (o *Operations) overview(c fiber.Ctx) error {
	q := dbgen.New(o.Pool)
	counts, err := q.ManagementOverviewCounts(c.Context())
	if err != nil {
		return failure(err)
	}
	recent, err := q.ManagementRecentIntake(c.Context())
	if err != nil {
		return failure(err)
	}
	states, err := q.ManagementIntakeRecentCounts(c.Context())
	if err != nil {
		return failure(err)
	}
	status, err := o.health(c.Context())
	if err != nil {
		return failure(err)
	}
	return c.JSON(fiber.Map{"region": o.Region, "site_assets": o.Assets != nil, "counts": counts, "recent_intake": recent, "recent_counts": states, "collection": status})
}

type intakeInput struct {
	AppID  string `json:"app_id"`
	Action string `json:"action"`
}

type pauseInput struct {
	Paused   *bool `json:"paused"`
	Expected *bool `json:"expected_paused"`
}

func (o *Operations) acquisition(g fiber.Router) {
	g.Get("/operations/discovery", func(c fiber.Ctx) error {
		if o.Discovery == nil {
			return c.JSON(fiber.Map{"enabled": false})
		}
		state, err := o.Discovery.Status(c.Context())
		if err != nil {
			return failure(err)
		}
		return c.JSON(fiber.Map{"enabled": true, "state": state, "budgets": o.Discovery.Budgets()})
	})
	g.Post("/operations/discovery", editOnly, func(c fiber.Ctx) error {
		if o.Discovery == nil {
			return fiber.ErrServiceUnavailable
		}
		var in pauseInput
		if err := body(c, &in); err != nil {
			return err
		}
		if in.Paused == nil || in.Expected == nil || *in.Paused == *in.Expected {
			return fiber.ErrBadRequest
		}
		action := "discovery.resume"
		if *in.Paused {
			action = "discovery.pause"
		}
		if err := o.Discovery.ManagePause(c.Context(), *in.Paused, in.Expected, receipt(c, action)); err != nil {
			return failure(err)
		}
		return c.JSON(catalog.ManagementResult{Target: "steam_discovery"})
	})
	g.Get("/operations/intake", func(c fiber.Ctx) error {
		values, err := search(c, "state", "app_id")
		if err != nil {
			return err
		}
		key := values.Get("app_id")
		if key != "" && !validAppID(key) {
			return fiber.ErrBadRequest
		}
		rows, err := dbgen.New(o.Pool).ManagementIntakeRows(c.Context(), dbgen.ManagementIntakeRowsParams{State: values.Get("state"), AppID: key})
		if err != nil {
			return failure(err)
		}
		return c.JSON(fiber.Map{"enabled": o.Intake != nil, "items": rows})
	})
	g.Post("/operations/intake", editOnly, func(c fiber.Ctx) error {
		if o.Intake == nil {
			return fiber.ErrServiceUnavailable
		}
		var in intakeInput
		if err := body(c, &in); err != nil {
			return err
		}
		if !validAppID(in.AppID) || (in.Action != "submit" && in.Action != "retry") {
			return fiber.ErrBadRequest
		}
		if err := o.Intake.ManageIntake(c.Context(), in.AppID, in.Action == "retry", receipt(c, "intake."+in.Action)); err != nil {
			return failure(err)
		}
		return c.JSON(catalog.ManagementResult{Target: in.AppID})
	})
	g.Get("/operations/collection", func(c fiber.Ctx) error {
		h, err := o.health(c.Context())
		if err != nil {
			return failure(err)
		}
		return c.JSON(h)
	})
	g.Post("/operations/collection/reconcile", editOnly, func(c fiber.Ctx) error {
		if !o.ReconcileEnabled {
			return fiber.ErrServiceUnavailable
		}
		var in struct{}
		if err := body(c, &in); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
		defer cancel()
		// The remote write has its own durable reconciliation protocol. This receipt
		// is appended only after that protocol reports a completed pass.
		audit := receipt(c, "collection.reconcile")
		audit.Target = "collection"
		if err := o.Collector.ManageReconcile(ctx, audit); err != nil {
			return failure(err)
		}
		return c.JSON(catalog.ManagementResult{Target: audit.Target})
	})
}

func validAppID(s string) bool {
	n, err := strconv.ParseUint(s, 10, 32)
	return err == nil && n != 0 && strconv.FormatUint(n, 10) == s
}
