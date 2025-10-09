# Venue Description Guidelines

This document provides comprehensive guidelines for writing and validating venue descriptions. AI uses these rules to evaluate description quality and conformance.

---

## Description Structure

**General pattern**: General features + vegan selection + additional useful information (if any)

**Example**:
> A bakery and café in a boutique house, which offers a selection of vegan pastries, cakes, various cookies, as well as made-in-house yogurt bowls, smoothie bowls, and cold-pressed juices. Hosts different community events on Wednesdays. Pet-friendly.

---

## General Guidelines

### Length
- Keep descriptions reasonably short: **2-6 sentences**
- Avoid overly brief descriptions (< 50 characters)
- Avoid excessively long descriptions (> 300 characters)

### Non-Veg Restaurant Rule (CRITICAL)
- For **restaurants and food trucks that serve meat or fish**:
    - Check BOTH the data flags (vegonly, vegan) AND the description content
    - **If vegonly=1 and vegan=1**: Venue is 100% VEGAN - DO NOT add "Serves meat" or "Serves fish"
    - **If vegonly=1 and vegan=0**: Venue is 100% VEGETARIAN - DO NOT add "Serves meat" (but may add "Serves fish" if pescatarian)
    - **If vegonly=0**: Venue serves meat/fish - MUST start with one of:
        - "Serves meat, vegan options available, "
        - "Serves meat, vegetarian options available, "
        - "Serves meat, vegan and vegetarian options available, "
        - "Serves fish, vegan options available, " (for pescatarian)
        - "Serves fish, vegetarian options available, " (for pescatarian)
    - **IMPORTANT**: If description mentions meat/fish dishes BUT venue data flags show vegonly=1 and / or vegan=0 or vegan=1, this is inconsistent - keep the "Serves meat" or "Serves fish" prefix based on description content and flag for manual review
    - Coffee shops and tea houses: DO NOT require "Serves meat" prefix
- Avoid repeating the word "options" in the remaining description

### Tone and Style
- **Neutral and factual** - not a biased endorsement or review
- **Remove promotional language and biased words**:
    - Quality words: great food, delicious, tasty, extremely tasty, excellent, amazing, wonderful, fantastic, superb, outstanding
    - Value words: great price, affordable, cheap, expensive, best value
    - Endorsements: best, the only one, highly recommended, come check us out, favorite, must-try
    - Replace with neutral descriptions: "wide selection", "various options", "menu includes"
- **Third person only** - change "we have" to "has", "you can choose" to "guests can choose"
- **Avoid pronouns**: Don't use "you", "they", "we" (see Preferred Writing Style section below)

### Content Rules
- **Do NOT include**:
  - Full menu or too many menu-specific dishes
  - Animal abuse mentions ("free-range eggs", "organic beef", "grass-fed beef")
  - Exact prices (subject to change)
  - Links to other websites (unless absolutely necessary)
  - Comments over a year old
  - Operating hours, open days, or closed days (these are displayed separately)

- **DO include**:
  - Relevant keywords (e.g., "candy" for candy shop, "noodles" for noodle restaurant)
  - A few interesting menu items (not the whole menu)

### Grammar and Punctuation
- **Remove exclamation marks**
- **Avoid second/third person**: "you can choose" → "guests can choose"
- **Change owner language to third person**: "we have" → "has"
- **Use "it" instead of "they"** for restaurants/stores
- **Write numbers as words**: Numbers should be written out as words (e.g., "5" → "five", "10" → "ten")
  - Apply to numbers 1-20 at minimum
  - Exceptions: years, dates, addresses
- **Do NOT capitalize dish names** unless they contain proper nouns (e.g., French fries, Impossible burger)
- **Do NOT use ALL CAPS** unless instructed
- **One space between sentences** (not two)

### Formatting
- **No blank spaces at the end** of description
- **No line spacing/paragraph breaks** unless bilingual description
- **Bilingual format**:
  - English description first
  - New line, then double dash `--`
  - Second language description on following line

### Word Choices
- Use **"house-made"** or **"made in-house"** (NOT "home-made")
- **Date format**: Use abbreviated month + full year
  - Examples: "Jan 2024", "Apr 2017", "Dec 2020"

---

## Additional Venue Info

**After menu info, include the following (only if applicable)**:

1. **Designations**: Non smoking. Child-friendly. Companion animal-friendly. Cash-free.
2. **Age restrictions**: "Must be 21+ years old to enter."
3. **Directions** (if needed): "Directions: Take bus #5 to Westside Station, exit State Street and walk south for 3 blocks, and the restaurant will be on the right side."
4. **Continuity info**: Previous names and locations
5. **Temporary/unconfirmed notes** (at end): "Note: take-out only as of Sep 2021" or "Note: HappyCow is unable to confirm if this store is fully vegan - please confirm and send updates."

---

## Preferred Writing Style

### Use Specific Nouns Instead of Pronouns

❌ **Avoid**: "You can choose from…"
✅ **Use** (you can replace vegan with vegetarian / guests depending on available options / description):
- Vegans can choose from…
- Vegan choices are…
- Vegan items include…
- Vegan selection includes…

### Omit Pronouns for Restaurants

❌ **Avoid**: "They offer many vegan dishes…"
✅ **Use**:
- There are many vegan dishes…
- Offers many vegan dishes…
- Many vegan dishes are available…
- Has many choices for vegans…
- Serves various vegan dishes…

### Customization Language

**Exception**: Pronouns OK in adjectives like "build-your-own" and "create-your-own"

**Alternative adjectives**:
- customizable / customisable
- custom-made
- made-to-order
- personalized / personalised
- tailored
- specially made

### Avoid Repetitive Words

❌ **Bad Example**:
> Serves meat, vegan options available. Chinese fusion restaurant that offers vegan options for most dishes. You can choose to replace non-vegan items with tofu or mock chicken and vegetables. All sauce options are vegan apart from honey and chili sauce.

✅ **Good Example**:
> Serves meat, vegan options available. Chinese fusion restaurant that can make most dishes vegan using tofu or mock chicken and vegetables. All sauces are vegan apart from honey and chili.

---

## Good Description Examples

### Airport Venue
**Business Name Format**: `MUC - Oliva - T2` (Airport code – Venue Name – Terminal)

> Serves meat, vegan options available. A Turkish restaurant that offers vegan choices such as pea protein kebab with vegetables, soup, and salad.

---

### Cloud Kitchen
> Family-run kitchen serving house-made, sugar-free and gluten-free meals that focuses on a healthy and balanced lifestyle. The menu includes hot Buddha bowls, raw juices, soups, and desserts. Order for collection or via Deliveroo, Uber, or Just Eat. NOTE: This restaurant is delivery only. It is considered to have a "cloud" or "virtual" kitchen and may share a kitchen space or have an off-site kitchen.

---

### Vegan Restaurant
> An international vegan restaurant est. 2020, which features a wide selection of food and drinks, ranging from Asian style like pad thai with fried wonton to western style like pesto pasta and burgers. Also serves coffee, tea, espresso drinks, juices, and sweet treats. Casual and comfortable dining atmosphere.

---

### Veg-options Restaurant
> Serves meat, vegan options available. A Portuguese restaurant where the menu changes weekly, but always offers five to seven vegan dishes. Main dishes may include oyster mushroom calamari, tofu bacalhau, courgette fritters with a garlic and saffron aioli, and pan-fried organic lentils.

---

### Delivery
> Vegan fast food delivery known for its chicken-style 'crispy fried jackfruit' burgers and wingz with a variety of toppings. Also sells loaded fries, beers, and fritz kola. Order via WhatsApp or Just Eat for delivery only.

---

### Bakery
> Scottish craft vegan bakery that offers various kinds of bread, pastries, pies, and tray bakes, such as batteries, cinnamon buns, croissants, doughnuts, and millionaire shortbread. Also offers made-to-order celebration cakes.

---

### Bakery (Delivery only)
> A delivery-only vegan bakery that offers various kinds of bread, pastries, pies, and tray bakes, such as batteries, cinnamon buns, croissants, doughnuts, and millionaire shortbread. Also offers made-to-order celebration cakes.

---

### Ice cream
> An ice cream shop that offers vegan ice cream in a separate freezer. Also has sundaes, cookies, shakes, loaded cheesecakes with sauces and toppings, as well as hot drinks. Vegan ice cream flavors include Biscoff, vanilla, salted caramel, bubblegum, mango passionfruit, dark chocolate, and banana.

---

### Coffee/Tea
> A coffee shop with plant-based milk alternatives for coffee and other hot beverages. Occasionally offers vegan croissants as well as vegan-friendly specials. Also sells in-house ground coffee and fairtrade coffee beans.

---

### Juice Bar
> A certified organic juice bar that offers cold-pressed juices and shots, smoothies, kombucha, acai bowls, nice-cream bowls, as well as hot drinks. Provides carbon-neutral shipping and uses 100% recycled bottles.

---

### Catering
> Vegan catering for a variety of events, including retreats, celebrations, private dinners, and corporate events. The food is gourmet and seasonal. Provides menus on the website for breakfast, lunch, dinner, and small bites. Also offers vegan cooking classes.

---

### Market Vendor
> An all-vegan market stall which sells house-made food. Items include cheese, sandwiches, cake, sauces, fresh pasta, vegan meatballs and lots of other products. Uses seasonal local ingredients. Availability of items may be different for each day.

---

### Veg-store
> A health-focused vegan food store that sells a wide variety of products such as vegan burgers, sausages, tofu, granola, protein powder, coffee, snacks, and condiments.

---

### Spa
> Vegan spa and beauty salon that provides a variety of treatments such as massage, waxing, facials, eyelashes extension, manicure and pedicure. Uses all cruelty-free and mostly natural products.

---

### Professional
> Offers plant-based personal training, mindset coaching, and nutritional coaching, online or in-person. Specializes in working with vegan clients and those interested in transitioning to a vegan lifestyle.

---

### Food Truck
> Vegan food truck serving porridge and rice bowls in wintertime, smoothie bowls in the summer. Parks at Fehrberliner Wochenmarkt and Neuköllner Stoff. Check its webpage for location and schedule updates.

---

### Farmer's Market
> A marketplace with vendors selling regional, seasonal, and organic vegetables and fruits plus baked goods, oil, juices, plants, spreads, and other foods.

---

### Organization
> Volunteer vegan advocacy group that holds regular vegan information stalls in Glasgow, Edinburgh, Stirling, and other towns in the area. Check Facebook for upcoming events.

---

### Other
> Vegan and female-owned tattoo studio that focuses on tattoos of floral motifs. Uses only certified tattoo inks with plant-based ingredients, namely the brands Panthera Ink, Eternal Ink and World Famous Tattoo Ink. Advanced appointments only, book via the website.

---

### B&B
> Spa B&B that offers house-cooked vegan meals, massage and outdoor hot tub, and courses for plant-based cooking, digital photography, and knife making. Has 25 private acres with meadows, woodlands and river. Companion animal-friendly.

---

## AI Validation Checklist

When evaluating descriptions, AI should check for:

### ✅ Required Elements
- [ ] 2-6 sentences in length
- [ ] Non-veg restaurants start with "Serves meat, vegan options available,"
- [ ] Third person perspective (no "we", "you", "they")
- [ ] Neutral tone (no promotional language)
- [ ] Relevant keywords included
- [ ] Proper grammar and punctuation

### ❌ Issues to Flag
- [ ] First-person language ("we serve", "our menu")
- [ ] Second-person language ("you can choose")
- [ ] Promotional language ("best", "amazing", "delicious")
- [ ] Too short (< 50 characters)
- [ ] Too long (> 300 characters)
- [ ] Missing "Serves meat" prefix for non-veg restaurants
- [ ] Exclamation marks present
- [ ] ALL CAPS usage
- [ ] Mentions of animal products in favorable way
- [ ] Exact prices included
- [ ] Old comments (> 1 year old)

### Quality Scoring (0-10)

**9-10**: Perfect conformance
- Correct structure
- Neutral tone
- Third person
- No issues
- Appropriate length
- All required elements present

**7-8**: Good quality
- Minor issues (slightly vague, slightly too short/long)
- Mostly conforms to guidelines
- May have one small issue

**5-6**: Acceptable but needs improvement
- Missing some key info
- Slightly promotional tone
- Length issues
- Multiple minor issues

**3-4**: Poor quality
- First-person or second-person language
- Promotional tone
- Too vague or too brief
- Missing required elements

**0-2**: Unacceptable
- Multiple major violations
- Completely wrong format
- Highly promotional
- Inappropriate content

---

## Language-Specific Rules

### English Descriptions
- Use proper grammar and spelling
- Avoid abbreviations ("vegetarian" not "veg")
- Be specific about cuisine and offerings

### Non-English Descriptions
- If description is entirely in non-English language, **flag for translation**
- **Bilingual acceptable** if properly formatted:
  - English first
  - New line + `--`
  - Second language on next line

### Mixed Language (within same sentence)
- **Not acceptable** - flag as issue
- Should be either English only OR properly bilingual (separated format)

---

## Common Issues and Corrections

| Issue | Example | Correction |
|-------|---------|------------|
| First person | "We serve vegan burgers" | "Serves vegan burgers" |
| Second person | "You can choose from..." | "Vegans can choose from..." |
| Promotional | "Best vegan food in town!" | "Vegan restaurant serving..." |
| Exclamation marks | "Amazing selection!" | "Wide selection" |
| Missing prefix | [Non-veg] "Restaurant offering vegan dishes" | "Serves meat, vegan options available. Restaurant offering vegan dishes" |
| "They" for venue | "They offer vegan pizza" | "Offers vegan pizza" or "It offers vegan pizza" |
| "Home-made" | "Home-made pasta" | "House-made pasta" or "Made in-house pasta" |
| Capitalized dishes | "Vegan Burger and Fries" | "Vegan burger and fries" |
| Proper noun OK | "french fries" | "French fries" (proper noun - France) |
| Numbers as digits | "Offers 5 vegan dishes" | "Offers five vegan dishes" |
| Hours in description | "Open 9am-5pm daily" | Remove (hours displayed separately) |

---

## Edge Cases

### Multiple Language Descriptions
Format:
```
English description here.
--
中文描述在這裡。
```

### Cloud Kitchen
Always append:
> This restaurant is takeaway and delivery only. It is considered to have a virtual kitchen and may share a kitchen space or have an off-site kitchen.

### Temporary Changes
Use "Note:" at the end:
> Note: take-out only as of Sep 2021.

### Unconfirmed Information
> Note: HappyCow is unable to confirm if this store is fully vegan - please confirm and send updates.

---

## Testing Your Description

Ask yourself:
1. Is it 2-6 sentences?
2. Does it start with "Serves meat..." if non-veg?
3. Is it third person (no "we", "you", "they")?
4. Is it neutral (no "best", "amazing")?
5. Are relevant keywords included?
6. No exclamation marks?
7. Specific dishes mentioned (but not full menu)?
8. Additional info included (pet-friendly, etc.)?

If all YES → likely quality score 8-10
If any NO → quality score will be lower, with specific issues flagged
