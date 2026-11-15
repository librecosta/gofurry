package legacyimport

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"slices"
	"strconv"
	"strings"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
)

type key struct {
	Table string  `json:"table"`
	IDs   []int64 `json:"key"`
}

func (k key) String() string {
	s := k.Table
	for _, id := range k.IDs {
		s += "/" + strconv.FormatInt(id, 10)
	}
	return s
}
func makeKey(table string, ids ...int64) key { return key{table, ids} }
func (k key) valid() bool {
	return len(k.IDs) > 0 && !slices.ContainsFunc(k.IDs, func(id int64) bool { return id <= 0 })
}
func (k key) first() int64 { return k.IDs[0] }

const (
	sites         = "gfn_site"
	games         = "gfg_game"
	collections   = "gfg_game_collection"
	categories    = "gfg_tag_category"
	tags          = "gfg_tag"
	groups        = "gfn_site_group"
	groupMembers  = "gfn_site_group_map"
	tagMembers    = "gfg_game_tag"
	manualMembers = "gfg_game_collection_item"
	rules         = "gfg_game_collection_tag"
	exclusions    = "gfg_game_collection_exclusion"
)

func kind(table string) string {
	switch table {
	case sites:
		return "site"
	case games:
		return "game"
	case collections:
		return "collection"
	case categories:
		return "tag_category"
	case tags:
		return "tag"
	}
	return ""
}

type decision struct {
	Operation string         `json:"operation"`
	Subject   *key           `json:"subject,omitempty"`
	Subjects  []key          `json:"subjects,omitempty"`
	Reviewer  string         `json:"reviewer"`
	Reason    string         `json:"reason"`
	Evidence  string         `json:"evidence"`
	Value     jsontext.Value `json:"value"`
}

type decisionFile struct {
	Version   int        `json:"decision_schema_version"`
	Content   string     `json:"content_sha256"`
	Decisions []decision `json:"decisions"`
}

const maxPlanBytes = 128 << 20

func loadDecisions(c Config) ([]decision, string, error) {
	if c.DecisionsFile == "" {
		return nil, digest(encode(decisionFile{1, c.Content, []decision{}})), nil
	}
	raw, err := readFile(c.DecisionsFile, 32<<20)
	if err != nil {
		return nil, "", err
	}
	return parseDecisions(raw, c.Content)
}

func parseDecisions(raw []byte, content string) ([]decision, string, error) {
	var file decisionFile
	if decode(raw, &file) != nil || file.Version != 1 || file.Content != content || file.Decisions == nil || len(file.Decisions) > 250000 {
		return nil, "", errors.New("invalid or incompatible decision file")
	}
	return file.Decisions, digest(raw), nil
}

func decode(raw []byte, into any) error {
	return json.Unmarshal(raw, into, json.RejectUnknownMembers(true))
}

type linkValue struct {
	ID       uuid.UUID `json:"target_id"`
	Kind     string    `json:"kind"`
	PublicID string    `json:"public_id"`
	Revision int64     `json:"revision"`
}
type groupValue struct {
	Category string   `json:"category,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}
type taxonomyValue struct {
	Primary   string   `json:"primary,omitempty"`
	Secondary []string `json:"secondary,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}
type addressValue struct {
	Primary    *key  `json:"primary,omitempty"`
	Alternates []key `json:"alternates,omitempty"`
}
type relationValue struct {
	Ignore   bool   `json:"ignore,omitempty"`
	Role     string `json:"role,omitempty"`
	Position int    `json:"position,omitempty"`
}
type publishValue struct {
	SFW               bool `json:"sfw,omitempty"`
	RelationsApproved bool `json:"relations_approved,omitempty"`
}
type proof struct {
	Actor string
	Ref   string
}

func decisionProof(d decision) proof {
	return proof{d.Reviewer, digest(encode(d))}
}

type reviewed struct {
	slug         string
	code         string
	link         *linkValue
	alias        bool
	linkProof    proof
	group        *groupValue
	taxonomy     *taxonomyValue
	addresses    *addressValue
	sources      *[]catalog.Source
	relation     *relationValue
	logo         string
	publish      *publishValue
	publishProof proof
	refs         []string
}

func validTaxonomy(v taxonomyValue) bool {
	seen := map[string]bool{}
	if v.Primary != "" {
		if !catalog.NavigationCategory(v.Primary) {
			return false
		}
		seen[v.Primary] = true
	}
	if len(v.Secondary) > 2 || len(v.Secondary) > 0 && v.Primary == "" {
		return false
	}
	for _, c := range v.Secondary {
		if seen[c] || !catalog.NavigationCategory(c) {
			return false
		}
		seen[c] = true
	}
	return validTags(v.Tags)
}
func validTags(tags []string) bool {
	used := map[string]bool{}
	for _, t := range tags {
		if used[t] || !catalog.NavigationTag(t) {
			return false
		}
		used[t] = true
	}
	return len(tags) <= 8
}
func (v *reviewed) take(table string, d decision) bool {
	t := table
	switch d.Operation {
	case "supply_slug":
		return slices.Contains([]string{sites, games, collections}, t) && decode(d.Value, &v.slug) == nil && v.slug != ""
	case "resolve_tag_code":
		return (t == tags || t == categories) && decode(d.Value, &v.code) == nil && codePattern.MatchString(v.code)
	case "link_existing", "alias_existing":
		if kind(t) == "" || v.link != nil || d.Operation == "alias_existing" && t != sites && t != games {
			return false
		}
		var link linkValue
		if decode(d.Value, &link) != nil || link.Kind != kind(t) || uuidVersion(link.ID) != 7 || link.Revision < 1 {
			return false
		}
		if (t == sites || t == games || t == collections) && !catalog.ValidPublicID(link.PublicID) || (t == tags || t == categories) && link.PublicID != "" {
			return false
		}
		v.link, v.alias, v.linkProof = &link, d.Operation == "alias_existing", decisionProof(d)
		return true
	case "map_group":
		switch t {
		case groups:
			var value groupValue
			if decode(d.Value, &value) != nil || value.Category != "" && !catalog.NavigationCategory(value.Category) ||
				!validTags(value.Tags) || value.Category == "" && len(value.Tags) == 0 {
				return false
			}
			v.group = &value
			return true
		case sites:
			var value taxonomyValue
			if decode(d.Value, &value) != nil || !validTaxonomy(value) {
				return false
			}
			v.taxonomy = &value
			return true
		}
	case "select_address":
		return t == sites && decode(d.Value, &v.addresses) == nil && v.addresses != nil &&
			len(v.addresses.Alternates) <= 15
	case "select_source":
		return t == games && decode(d.Value, &v.sources) == nil && v.sources != nil &&
			*v.sources != nil && len(*v.sources) <= 16
	case "resolve_relation":
		if !slices.Contains([]string{tagMembers, manualMembers, rules, exclusions, groupMembers}, t) {
			return false
		}
		if decode(d.Value, &v.relation) != nil || v.relation == nil {
			return false
		}
		r := v.relation
		if r.Ignore {
			return r.Role == "" && r.Position == 0
		}
		return t == tagMembers && validRole(r.Role) && r.Position == 0 ||
			t == manualMembers && r.Role == "" && r.Position >= 1 && r.Position <= 512
	case "allow_logo":
		return t == sites && decode(d.Value, &v.logo) == nil && digestPattern.MatchString(v.logo)
	case "allow_publish":
		if decode(d.Value, &v.publish) != nil || v.publish == nil {
			return false
		}
		v.publishProof = decisionProof(d)
		return (t == sites || t == games) && v.publish.SFW && !v.publish.RelationsApproved ||
			t == collections && !v.publish.SFW && v.publish.RelationsApproved
	}
	return false
}

func reviewDecisions(ds []decision, rows map[string]seedRow) (map[string]*reviewed, error) {
	result := map[string]*reviewed{}
	seen := map[[2]string]bool{}
	for _, d := range ds {
		subjects := d.Subjects
		if d.Subject != nil && len(subjects) == 0 {
			subjects = []key{*d.Subject}
		} else if d.Subject != nil {
			return nil, errors.New("decision must use one subject form")
		}
		if len(subjects) == 0 || len(subjects) > 10000 ||
			!(catalog.PublicationDecision{Actor: d.Reviewer, Reason: d.Reason}).Valid() ||
			strings.TrimSpace(d.Evidence) == "" || len(d.Evidence) > 400 {
			return nil, errors.New("invalid decision subjects or reviewer evidence")
		}
		for _, k := range subjects {
			s := k.String()
			sig := [2]string{d.Operation, s}
			if seen[sig] || rows[s].key.Table == "" || !k.valid() {
				return nil, errors.New("duplicate or unknown decision subject")
			}
			seen[sig] = true
			v := result[s]
			if v == nil {
				v = &reviewed{}
				result[s] = v
			}
			if !v.take(k.Table, d) {
				return nil, errors.New("invalid decision operation or typed value")
			}
			v.refs = append(v.refs, decisionProof(d).Ref)
		}
	}

	primary := map[[2]string]bool{}
	for _, v := range result {
		if v.link != nil && !v.alias {
			k := [2]string{v.link.Kind, v.link.ID.String()}
			if primary[k] {
				return nil, errors.New("multiple primary adoptions require explicit alias decisions")
			}
			primary[k] = true
		}
		if v.link != nil && (v.publish != nil || v.addresses != nil || v.logo != "" || v.slug != "" || v.code != "" || v.taxonomy != nil) {
			return nil, errors.New("editorial decisions conflict with adopting an existing target")
		}
		slices.Sort(v.refs)
	}
	return result, nil
}

func uuidVersion(id uuid.UUID) byte { return id[6] >> 4 }
