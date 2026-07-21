<script setup lang="ts">
  import MarkdownIt from "markdown-it";
  import { imgSize } from "@mdit/plugin-img-size";
  import DOMPurify from "dompurify";

  type Props = {
    source?: string | null;
    /**
     * Render single newlines as line breaks (markdown-it's `breaks`). Opted
     * into by description renders so entered line breaks display as entered
     * instead of collapsing into a run-on paragraph.
     */
    breaks?: boolean;
  };

  const props = withDefaults(defineProps<Props>(), {
    source: null,
    breaks: false,
  });

  const md = new MarkdownIt({
    html: true,
    breaks: props.breaks,
    linkify: true,
    typographer: true,
  }).use(imgSize);

  const raw = computed(() => {
    const html = md.render(props.source || "").replace(/\n$/, ""); // remove trailing newline
    return DOMPurify.sanitize(html);
  });
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -->
  <div class="markdown prose text-wrap break-words" v-html="raw" />
</template>

<style scoped>
  * {
    word-wrap: break-word; /*Fix for long words going out of emelent bounds and issue #407 */
    overflow-wrap: break-word; /*Fix for long words going out of emelent bounds and issue #407 */
  }
  .markdown {
    max-width: 100%;
    overflow: hidden; /*Fix for long words going out of emelent bounds and issue #407 */
  }
</style>
