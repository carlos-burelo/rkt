import { defineCollection } from "astro:content";
import { glob } from "astro/loaders";

// Única fuente pública: site/docs/*.md (ruta /docs/*).
const docs = defineCollection({
  loader: glob({ pattern: "**/*.md", base: "./docs" }),
});

export const collections = { docs };
