# Venue Name Translation Guidelines

## Core Principle

**All non-English venue names MUST include both the English/Romanization AND the native script, separated by a dash.**

**Format**: `[English/Romanization] - [Native Script]` or `[English/Romanization] -[Native Script]`

---

## Translation Format Types

### 1. Phonetic Pronunciation (Preferred for names without official English)

**When to use**: The venue has no official English name

**Pattern**: `Romanized Pronunciation -Native Script`

**Example**:
```
Dīngxiāng Dòuhuā Sùshí Shuǐjiǎo -丁香豆花素食水餃
```

**Characteristics**:
- Uses phonetic romanization (pinyin for Chinese, romanized Hangul for Korean, etc.)
- May include tone marks (ā, é, ǐ, ō, ū) for Chinese
- Pronunciation-based, not meaning-based

---

### 2. Official English Name (Preferred if available)

**When to use**: Business has an official/registered English name

**Pattern**: `Official English Business Name - Native Script`

**Example**:
```
Action Vege Cafe & Live Music - 黑炫蔬食音樂咖啡館
```

**Characteristics**:
- Business owner has chosen/registered an English name
- May be creative, not a direct translation
- Official branding includes both languages

**Priority**: **Use official English name if it exists** (preferred over phonetic)

---

### 3. Literal Translation (Avoid if possible)

**When to use**: ONLY when neither official English name NOR phonetic romanization is available

**Pattern**: `English Translation - Native Script`

**Example**:
```
Buddha Vegetarian - 佛素食
```

**Note**: Avoid literal translation and use the official English name if applicable, or phonetic name.

**Priority**: Literal translation is **last resort** - prefer official English or phonetic

---

## Priority Order

When formatting a non-English venue name, use this priority:

1. **Official English Name** (if the business uses one) - e.g., `Action Vege Cafe - 黑炫蔬食`
2. **Phonetic Pronunciation** (romanization) - e.g., `Dīngxiāng Dòuhuā -丁香豆花`
3. **Literal Translation** (only if nothing else available) - e.g., `Buddha Vegetarian - 佛素食`

---

## Language-Specific Romanization

### Chinese (中文)
- **System**: Pinyin (Mandarin)
- **With tone marks**: `Dīngxiāng Dòuhuā` (preferred)
- **Without tone marks**: `Dingxiang Douhua` (acceptable)
- **Examples**: 素食 → `Sùshí` or `Sushi`

### Korean (한글)
- **System**: Revised Romanization of Korean
- **Examples**:
  - 비빔밥 → `Bibimbap`
  - 김치 → `Kimchi`
  - 사찰음식 → `Sachal Eumsik`

### Japanese (日本語)
- **System**: Hepburn romanization
- **Examples**:
  - やさい → `Yasai`
  - ラーメン → `Ramen`
  - 精進料理 → `Shojin Ryori`

### Thai (ไทย)
- **System**: Royal Thai General System (RTGS)
- **Romanize** based on Thai phonetics

---

## Format Validation Rules

### ✅ Correct Formats

```
Dīngxiāng Dòuhuā -丁香豆花
Action Vege Cafe - 黑炫蔬食音樂咖啡館
Buddha Vegetarian - 佛素食
Green Tea House - 綠茶館
Loving Hut - 愛家
```

### ❌ Incorrect Formats

```
丁香豆花 (native only, missing English/romanization)
Dingxiang Douhua (English/romanization only, missing native script)
Action Vege (黑炫蔬食) (wrong separator - parentheses)
黑炫蔬食 - Action Vege (reversed - native first instead of English)
Action / Vege - 黑炫 (wrong separator - slash)
Action Vege Cafe—黑炫蔬食 (wrong separator - em dash instead of hyphen-dash)
```

---

## Separator Rules

### ✅ Accepted Separators
- **Hyphen-dash with space**: ` - ` (most common)
- **Hyphen-dash without space before**: `-` (acceptable, e.g., `Name -Native`)
- **Space-dash-space**: ` - ` (preferred for readability)

### ❌ Unacceptable Separators
- Parentheses: `()`
- Slash: `/`
- Comma: `,`
- Em dash: `—`
- No separator at all (e.g., `Name Chinese中文`)

---

## Character Detection

### Chinese (中文)
**Character ranges**: U+4E00–U+9FFF (CJK Unified Ideographs)
**Examples**: 中, 文, 素, 食, 餐, 廳, 豆, 腐

### Korean (한글)
**Character ranges**: U+AC00–U+D7AF (Hangul Syllables), U+1100–U+11FF (Hangul Jamo)
**Examples**: 한, 글, 김, 치, 비, 빔, 밥

### Japanese (日本語)
**Character ranges**:
- Hiragana: U+3040–U+309F (あ, い, う)
- Katakana: U+30A0–U+30FF (ア, イ, ウ)
- Kanji: U+4E00–U+9FFF (shared with Chinese)
**Examples**: やさい, ラーメン, 精進料理

### Thai (ไทย)
**Character ranges**: U+0E00–U+0E7F
**Examples**: อ, า, ห, ร, ม, ง, ส, ว, ร, ต

---

## AI Validation Instructions

When validating a venue name, follow these steps:

### 1. Detect Non-English Characters
- Scan name for Chinese/Korean/Japanese/Thai characters
- If **no non-English characters** found → `format: "correct"`, `translation_type: "none"`, `original_detected: "none"`
- If **non-English characters found** → proceed to step 2

### 2. Check for Dash Separator
- Look for ` - ` or `-` between English and native parts
- If **no dash found** → determine if missing English or missing native

### 3. Determine Format Status

**If dash present and both parts exist**:
- `format: "correct"`
- Proceed to determine translation type

**If only native script (no English/romanization)**:
- `format: "needs_translation"`
- `suggested_name: "[Romanized Name] -[Native Script]"`

**If only English/romanization (no native script)**:
- `format: "missing_native"`
- `suggested_name: "[English Name] - [Native Script]"`

**If wrong separator or reversed order**:
- `format: "incorrect"`
- `suggested_name: "[English Name] - [Native Script]"` (corrected)

### 4. Determine Translation Type

Analyze the English portion:

**Official English Name** (`translation_type: "official"`):
- Proper English words that form a business name
- Creative or trademarked names
- Examples: "Action Vege Cafe", "Loving Hut", "Green Common"

**Phonetic Romanization** (`translation_type: "phonetic"`):
- Uses romanization systems (Pinyin, Revised Romanization, Hepburn)
- May include tone marks (ā, é, ǐ)
- Pronunciation-based, not meaning-based
- Examples: "Dīngxiāng Dòuhuā", "Gogung", "Yasai Ramen"

**Literal Translation** (`translation_type: "literal"`):
- Direct English translation of native name
- Conveys meaning, not pronunciation
- Examples: "Buddha Vegetarian", "Green Tea House", "Tofu Factory"

**None** (`translation_type: "none"`):
- Fully English name, no translation needed
- Example: "Green Valley Restaurant"

### 5. Set Language Detection

Based on character ranges detected:
- Chinese characters → `original_detected: "zh"`
- Korean Hangul → `original_detected: "ko"`
- Japanese characters → `original_detected: "ja"`
- Thai characters → `original_detected: "th"`
- No non-English → `original_detected: "none"`

### 6. Set Native Script Flag

- If any non-English characters present anywhere in name → `has_native_script: true`
- If purely English → `has_native_script: false`

---

## AI Output Examples

### Example 1: Correct Phonetic Format
**Input**: `Dīngxiāng Dòuhuā -丁香豆花`

**AI Output**:
```json
{
  "format": "correct",
  "translation_type": "phonetic",
  "suggested_name": "",
  "original_detected": "zh",
  "has_native_script": true
}
```

---

### Example 2: Correct Official English Format
**Input**: `Action Vege Cafe & Live Music - 黑炫蔬食音樂咖啡館`

**AI Output**:
```json
{
  "format": "correct",
  "translation_type": "official",
  "suggested_name": "",
  "original_detected": "zh",
  "has_native_script": true
}
```

---

### Example 3: Correct Literal Translation Format
**Input**: `Buddha Vegetarian - 佛素食`

**AI Output**:
```json
{
  "format": "correct",
  "translation_type": "literal",
  "suggested_name": "",
  "original_detected": "zh",
  "has_native_script": true
}
```

---

### Example 4: Missing English Translation
**Input**: `丁香豆花`

**AI Output**:
```json
{
  "format": "needs_translation",
  "translation_type": "none",
  "suggested_name": "[Romanized Name] -丁香豆花",
  "original_detected": "zh",
  "has_native_script": true
}
```

---

### Example 5: Missing Native Script
**Input**: `Dingxiang Douhua`

**AI Output**:
```json
{
  "format": "missing_native",
  "translation_type": "phonetic",
  "suggested_name": "Dingxiang Douhua - [Native Script]",
  "original_detected": "none",
  "has_native_script": false
}
```

---

### Example 6: Wrong Separator (Parentheses)
**Input**: `Action Vege Cafe (黑炫蔬食)`

**AI Output**:
```json
{
  "format": "incorrect",
  "translation_type": "official",
  "suggested_name": "Action Vege Cafe - 黑炫蔬食",
  "original_detected": "zh",
  "has_native_script": true
}
```

---

### Example 7: Reversed Order
**Input**: `黑炫蔬食 - Action Vege Cafe`

**AI Output**:
```json
{
  "format": "incorrect",
  "translation_type": "official",
  "suggested_name": "Action Vege Cafe - 黑炫蔬食",
  "original_detected": "zh",
  "has_native_script": true
}
```

---

### Example 8: Fully English Name (No Translation Needed)
**Input**: `Green Valley Restaurant`

**AI Output**:
```json
{
  "format": "correct",
  "translation_type": "none",
  "suggested_name": "",
  "original_detected": "none",
  "has_native_script": false
}
```

---

## Edge Cases

### Multiple Scripts in One Name
**Example**: `Korean Tofu - 韓國豆腐 - 한국두부`

**Handling**: Accept if well-formatted; note multiple languages in validation

### Spacing Variations
- `Name - Native` ✅ (preferred)
- `Name -Native` ✅ (acceptable)
- `Name-Native` ⚠️ (acceptable but less readable)
- `Name  -  Native` ⚠️ (extra spaces, suggest fixing)

### Mixed English and Native in Single Part
**Example**: `Veggie 素食 Restaurant`

**Handling**: Flag as `incorrect` - should be either fully English OR use dash format

### Abbreviated Native Script
**Example**: `Green Tea - 綠茶` (abbreviated from 綠茶館)

**Handling**: Acceptable if both parts present with dash

---

## Quality Indicators

### ✅ High Quality Name
- Official English name OR proper phonetic romanization
- Native script included
- Proper dash separator
- Correct order (English first)
- Clear and readable

### ⚠️ Medium Quality Name
- Correct format but could be improved
- Literal translation instead of official/phonetic
- Minor spacing issues
- Acceptable but not ideal

### ❌ Low Quality Name
- Wrong separator or format
- Missing English OR native part
- Reversed order
- Needs immediate correction

---

## Summary Checklist

When AI validates a venue name:

- [ ] Detect non-English characters (Chinese/Korean/Japanese/Thai)
- [ ] Check for dash separator (` - ` or `-`)
- [ ] Verify English/romanization comes FIRST
- [ ] Verify native script comes AFTER dash
- [ ] Determine translation type (official > phonetic > literal)
- [ ] Set `format` status (correct/needs_translation/missing_native/incorrect)
- [ ] Provide `suggested_name` if format is not correct
- [ ] Set `original_detected` language (zh/ko/ja/th/none)
- [ ] Set `has_native_script` flag (true/false)

---

## Key Principles

1. **Official English name is preferred** over phonetic or literal
2. **Phonetic romanization is preferred** over literal translation
3. **Both English AND native must be present** (separated by dash)
4. **English/romanization comes FIRST**, native script comes SECOND
5. **Use dash separator** (` - ` or `-`), not parentheses, slashes, or commas
6. **Preserve original content** when suggesting corrections - only fix format, don't change the actual name
