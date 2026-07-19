<div align="center">
  <img src="/docs/src/assets/lilbox.svg" height="200"/>
</div>

<h1 align="center" style="margin-top: -10px;"> HomeBox </h1>
<p align="center" style="width: 100%;">
   <a href="https://homebox.software/en/">Docs</a>
   |
   <a href="https://demo.homebox.software">Demo</a>
   |
   <a href="https://discord.gg/aY4DCkpNA9">Discord</a>
</p>
<p align="center" style="width: 100%;">
    <img src="https://img.shields.io/github/check-runs/nstefanelli/homebox/main" alt="Github Checks"/>
    <img src="https://img.shields.io/github/license/nstefanelli/homebox"/>
    <img src="https://img.shields.io/weblate/progress/homebox?server=https%3A%2F%2Ftranslate.sysadminsmedia.com"/>
</p>
<p align="center" style="width: 100%;">
    <img src="https://img.shields.io/reddit/subreddit-subscribers/homebox"/>
    <img src="https://img.shields.io/mastodon/follow/110749314839831923?domain=infosec.exchange"/>
    <img src="https://img.shields.io/lemmy/homebox%40lemmy.world?label=lemmy"/>
</p>
<p align="center" style="width: 100%;">
  <a href="https://www.pikapods.com/pods?run=homebox"><img src="https://www.pikapods.com/static/run-button.svg"/></a>
</p>

## About this fork

This is a personal fork ([nstefanelli/homebox](https://github.com/nstefanelli/homebox)) of [sysadminsmedia/homebox](https://github.com/sysadminsmedia/homebox) v0.26.2, adding five feature sets on top of upstream:

- **Containers & totes** (`v0.26.2-containers.1`) — a `Container` entity type with nested contents, move/empty-container actions, batch container creation, and QR label queue printing for physical totes/bins.
- **AI add-by-photo** (v0.26.2-ai-icons.1) — photograph an item and a vision-LLM prefills name/description/manufacturer/model/tag hints for you to verify before saving; if a barcode is visible in the photo it routes to the existing UPC lookup instead. Provider-pluggable via `HBOX_AI_PROVIDER` / `HBOX_AI_BASE_URL` / `HBOX_AI_API_KEY` / `HBOX_AI_MODEL` / `HBOX_AI_TIMEOUT_SECONDS` (OpenAI-compatible APIs, including local Ollama, or Anthropic). Leaving it unconfigured fully hides the feature.
- **Entity icons** (v0.26.2-ai-icons.1) — customizable icons for locations and containers, with type-level defaults and per-entity overrides, rendered throughout the tree view, cards, selectors, and breadcrumbs.
- **Bulk tote cataloging** (v0.26.2-phase3.1) — photograph multiple items in a container, review AI-generated candidates per item with edit/check/skip/retry, and batch-create entities into the target container. Contents-snapshot photos stored with the container.
- **Integrations settings UI** (v0.26.2-phase3.1) — group-scoped database-backed AI provider, model, and barcode settings on the collection settings page, with environment variables as per-field fallback defaults, live-apply without restart, and test-connection endpoints for validation.

See `CHANGELOG.md` for release-by-release detail.

## What is HomeBox

HomeBox is the inventory and organization system built for the Home User! With a focus on simplicity and ease of use, Homebox is the perfect solution for your home inventory, organization, and management needs. While developing this project, We've tried to keep the following principles in mind:

- 🧘 _Simple but Expandable_ - Homebox is designed to be simple and easy to use. No complicated setup or configuration required. But expandable to whatever level of infrastructure you want to put into it.
- 🚀 _Blazingly Fast_ - Homebox is written in Go, which makes it extremely fast and requires minimal resources to deploy. In general, idle memory usage is less than 50MB for the whole container.
- 📦 _Portable_ - Homebox is designed to be portable and run on anywhere. We use SQLite and an embedded Web UI to make it easy to deploy, use, and backup.

### Key Features
- 📇 Rich Organization - Organize your items into categories, locations, and tags. You can also create custom fields to store additional information about your items.
- 🔍 Powerful Search - Quickly find items in your inventory using the powerful search feature.
- 📸 Image Upload - Upload images of your items to make it easy to identify them.
- 📄 Document and Warranty Tracking - Keep track of important documents and warranties for your items.
- 💰 Purchase & Maintenance Tracking - Track purchase dates, prices, and maintenance schedules for your items.
- 📱 Responsive Design - Homebox is designed to work on any device, including desktops, tablets, and smartphones.

## Screenshots
![Login Screen](.github/screenshots/1.png)
![Dashboard](.github/screenshots/2.png)
![Item View](.github/screenshots/3.png)
![Create Item](.github/screenshots/9.png)
![Search](.github/screenshots/8.png)

You can also try the demo instances of Homebox:
- [Demo](https://demo.homebox.software)
- [Nightly](https://nightly.homebox.software)

## Quick Start

This fork does not currently publish a verified prebuilt container image. Build it
from this repository so the custom container, AI, icon, cataloging, and integration
features are present:

```bash
git clone https://github.com/nstefanelli/homebox.git
cd homebox

docker build --pull --tag homebox-fork:local .

mkdir -p /path/to/data/folder
printf 'HBOX_AUTH_API_KEY_PEPPER=%s\n' "$(openssl rand -base64 48)" > .env
chmod 600 .env

docker run -d \
  --name homebox \
  --restart unless-stopped \
  --publish 3100:7745 \
  --env TZ=Europe/Bucharest \
  --env-file .env \
  --volume /path/to/data/folder/:/data \
  homebox-fork:local
```

Keep the generated `.env` file private and backed up. The API-key pepper is
required, must be at least 32 bytes, and must remain unchanged across restarts;
rotating it invalidates issued API keys.

The checked-in Compose file builds the regular image, reads that `.env` file,
and persists `/data` in the `homebox-data` named volume:

```bash
HBOX_BUILD_COMMIT="$(git rev-parse HEAD)" \
HBOX_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
docker compose up -d --build
```

To build the non-root or distroless variants, use `Dockerfile.rootless` or
`Dockerfile.hardened` and ensure a bind-mounted data directory is owned by
UID/GID `65532`. The upstream `sysadminsmedia/homebox` images do not contain this
fork's custom features.

[Full configuration reference](docs/src/content/docs/en/quick-start/configure/index.mdx)

<!-- CONTRIBUTING -->

## Contributing

Contributions are what make the open source community such an amazing place to learn, inspire, and create. Any contributions you make are **greatly appreciated**.

To get started with code based contributions, please see our [contributing guide](https://homebox.software/en/contribute/).

If you are not a coder and can't help translate, you can still contribute financially. Financial contributions help us maintain the project and keep demos running.

## Help us Translate
We want to make sure that Homebox is available in as many languages as possible. If you are interested in helping us translate Homebox, please help us via our [Weblate instance](https://translate.sysadminsmedia.com/projects/homebox/).

[![Translation status](https://translate.sysadminsmedia.com/widget/homebox/multi-auto.svg)](https://translate.sysadminsmedia.com/engage/homebox/)

## Credits
- Original project by [@hay-kot](https://github.com/hay-kot)
- Logo by [@lakotelman](https://github.com/lakotelman)

### Contributors
<a href="https://github.com/sysadminsmedia/homebox/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=sysadminsmedia/homebox" />
</a>
