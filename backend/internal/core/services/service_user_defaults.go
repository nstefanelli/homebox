package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
)

const defaultLocationGarage = "Garage"

// ensureDefaultEntityTypes guarantees a freshly created group has the two
// baseline entity types ("Item" and "Location"). The frontend create dialogs
// require selecting an existing type, so without these a brand-new group can't
// create items or locations at all. GetDefault creates each type if missing.
func ensureDefaultEntityTypes(ctx context.Context, repos *repo.AllRepos, gid uuid.UUID) error {
	for _, isLocation := range []bool{false, true} {
		if _, err := repos.EntityTypes.GetDefault(ctx, gid, isLocation); err != nil {
			return err
		}
	}
	return nil
}

func createDefaultLocations(ctx context.Context, repos *repo.AllRepos, gid uuid.UUID) (int, error) {
	nextAssetID, err := repos.Entities.GetHighestAssetID(ctx, gid)
	if err != nil {
		return 0, err
	}

	created := 0
	for _, location := range defaultLocations() {
		nextAssetID++
		location.AssetID = nextAssetID
		if _, err := repos.Entities.CreateContainer(ctx, gid, location); err != nil {
			return created, err
		}
		created++
	}

	return created, nil
}

func defaultLocations() []repo.EntityCreate {
	return []repo.EntityCreate{
		{
			Name: "Living Room",
		},
		{
			Name: defaultLocationGarage,
		},
		{
			Name: "Kitchen",
		},
		{
			Name: "Bedroom",
		},
		{
			Name: "Bathroom",
		},
		{
			Name: "Office",
		},
		{
			Name: "Attic",
		},
		{
			Name: "Basement",
		},
	}
}

func defaultTags() []repo.TagCreate {
	return []repo.TagCreate{
		{
			Name: "Appliances",
		},
		{
			Name: "IOT",
		},
		{
			Name: "Electronics",
		},
		{
			Name: "Servers",
		},
		{
			Name: "General",
		},
		{
			Name: "Important",
		},
	}
}
