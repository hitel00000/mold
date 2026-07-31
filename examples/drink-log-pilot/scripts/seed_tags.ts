// Idempotent Default Seed Tags Script (22 Real Production Korean Default Tags)
// Uses `slug` + `INSERT OR IGNORE` to guarantee 100% re-execution safety.
// Slug Generation Rule: 'tag_' + tag_group + '_' + lower(replace(label, ' ', '_'))

export interface DefaultTagSeed {
  slug: string;
  tag_group: 'taste' | 'aroma' | 'mood';
  label: string;
}

export const DEFAULT_22_TAGS: DefaultTagSeed[] = [
  // taste (7)
  { slug: 'tag_taste_달콤함', tag_group: 'taste', label: '달콤함' },
  { slug: 'tag_taste_깔끔함', tag_group: 'taste', label: '깔끔함' },
  { slug: 'tag_taste_드라이함', tag_group: 'taste', label: '드라이함' },
  { slug: 'tag_taste_산미', tag_group: 'taste', label: '산미' },
  { slug: 'tag_taste_감칠맛', tag_group: 'taste', label: '감칠맛' },
  { slug: 'tag_taste_묵직함', tag_group: 'taste', label: '묵직함' },
  { slug: 'tag_taste_부드러움', tag_group: 'taste', label: '부드러움' },

  // aroma (11)
  { slug: 'tag_aroma_과일향_(사과/청포도)', tag_group: 'aroma', label: '과일향 (사과/청포도)' },
  { slug: 'tag_aroma_과일향_(참외/멜론)', tag_group: 'aroma', label: '과일향 (참외/멜론)' },
  { slug: 'tag_aroma_과일향_(감귤/레몬)', tag_group: 'aroma', label: '과일향 (감귤/레몬)' },
  { slug: 'tag_aroma_과일향_(바나나)', tag_group: 'aroma', label: '과일향 (바나나)' },
  { slug: 'tag_aroma_과일향_(열대과일)', tag_group: 'aroma', label: '과일향 (열대과일)' },
  { slug: 'tag_aroma_꽃향', tag_group: 'aroma', label: '꽃향' },
  { slug: 'tag_aroma_곡물향/쌀향', tag_group: 'aroma', label: '곡물향/쌀향' },
  { slug: 'tag_aroma_유제품향/요거트', tag_group: 'aroma', label: '유제품향/요거트' },
  { slug: 'tag_aroma_풀향/삼나무', tag_group: 'aroma', label: '풀향/삼나무' },
  { slug: 'tag_aroma_향신료향', tag_group: 'aroma', label: '향신료향' },
  { slug: 'tag_aroma_숙성향', tag_group: 'aroma', label: '숙성향' },

  // mood (4)
  { slug: 'tag_mood_입문자_추천', tag_group: 'mood', label: '입문자 추천' },
  { slug: 'tag_mood_반주/식사용', tag_group: 'mood', label: '반주/식사용' },
  { slug: 'tag_mood_특별한_날', tag_group: 'mood', label: '특별한 날' },
  { slug: 'tag_mood_혼술', tag_group: 'mood', label: '혼술' },
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
