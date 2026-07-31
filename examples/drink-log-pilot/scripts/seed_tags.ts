// Idempotent Default Seed Tags Script (22 Default Tags)
// Uses `slug` + `INSERT OR IGNORE` to guarantee 100% re-execution safety.

export interface DefaultTagSeed {
  slug: string;
  tag_group: 'taste' | 'aroma' | 'mood';
  label: string;
}

export const DEFAULT_22_TAGS: DefaultTagSeed[] = [
  // Taste Group (8)
  { slug: 'tag_taste_fresh', tag_group: 'taste', label: 'Fresh' },
  { slug: 'tag_taste_fruity', tag_group: 'taste', label: 'Fruity' },
  { slug: 'tag_taste_sweet', tag_group: 'taste', label: 'Sweet' },
  { slug: 'tag_taste_dry', tag_group: 'taste', label: 'Dry' },
  { slug: 'tag_taste_rich', tag_group: 'taste', label: 'Rich' },
  { slug: 'tag_taste_crisp', tag_group: 'taste', label: 'Crisp' },
  { slug: 'tag_taste_smooth', tag_group: 'taste', label: 'Smooth' },
  { slug: 'tag_taste_acidic', tag_group: 'taste', label: 'Acidic' },

  // Aroma Group (7)
  { slug: 'tag_aroma_floral', tag_group: 'aroma', label: 'Floral' },
  { slug: 'tag_aroma_apple', tag_group: 'aroma', label: 'Apple' },
  { slug: 'tag_aroma_melon', tag_group: 'aroma', label: 'Melon' },
  { slug: 'tag_aroma_rice', tag_group: 'aroma', label: 'Steamed Rice' },
  { slug: 'tag_aroma_citrus', tag_group: 'aroma', label: 'Citrus' },
  { slug: 'tag_aroma_herbal', tag_group: 'aroma', label: 'Herbal' },
  { slug: 'tag_aroma_banana', tag_group: 'aroma', label: 'Banana' },

  // Mood Group (7)
  { slug: 'tag_mood_casual', tag_group: 'mood', label: 'Casual' },
  { slug: 'tag_mood_dinner', tag_group: 'mood', label: 'Dinner Party' },
  { slug: 'tag_mood_solo', tag_group: 'mood', label: 'Solo Drink' },
  { slug: 'tag_mood_celebration', tag_group: 'mood', label: 'Celebration' },
  { slug: 'tag_mood_gift', tag_group: 'mood', label: 'Gift Recommendation' },
  { slug: 'tag_mood_relaxed', tag_group: 'mood', label: 'Relaxed' },
  { slug: 'tag_mood_refreshing', tag_group: 'mood', label: 'Refreshing' },
];

export async function seedDefaultTags(db: D1Database): Promise<{ seeded: number; total: number }> {
  const now = new Date().toISOString();
  let seededCount = 0;

  for (const tag of DEFAULT_22_TAGS) {
    const res = await db
      .prepare(
        'INSERT OR IGNORE INTO "tags" ("slug", "owner_id", "drink_type", "tag_group", "label", "is_default", "created_at", "updated_at") VALUES (?, NULL, "sake", ?, ?, 1, ?, ?)'
      )
      .bind(tag.slug, tag.tag_group, tag.label, now, now)
      .run();

    if (res.meta.changes > 0) {
      seededCount++;
    }
  }

  return { seeded: seededCount, total: DEFAULT_22_TAGS.length };
}
