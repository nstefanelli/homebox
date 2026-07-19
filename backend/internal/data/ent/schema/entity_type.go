package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/schema/mixins"
)

// EntityType holds the schema definition for the EntityType entity.
type EntityType struct {
	ent.Schema
}

func (EntityType) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.BaseMixin{},
		mixins.DetailsMixin{},
		GroupMixin{ref: "entity_types"},
	}
}

func (EntityType) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entsql.Checks(map[string]string{
			"entity_types_container_requires_location": "NOT is_container OR is_location",
		}),
	}
}

// Fields of the EntityType.
func (EntityType) Fields() []ent.Field {
	return []ent.Field{
		field.Bool("is_location").
			Default(false),
		field.Bool("is_container").
			Default(false).
			Comment("Container types are movable holders (totes/bins); requires is_location"),
		field.String("icon").
			MaxLen(255).
			Optional(),
	}
}

// Edges of the EntityType.
func (EntityType) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("entities", Entity.Type).
			Annotations(entsql.Annotation{
				OnDelete: entsql.Restrict,
			}),
		edge.To("default_template", EntityTemplate.Type).
			Unique(),
	}
}
