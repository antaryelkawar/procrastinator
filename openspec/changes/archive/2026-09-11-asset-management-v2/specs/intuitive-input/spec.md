# intuitive-input — Delta

## Context: research-informed pattern choice (delta 2)

Following the user's rejection of old-style field-by-field forms ("no old-style forms. More intuitive input"), the pattern below was chosen from current UX research (websearcher, 2026-09-09). External sources looked at NB / patterns considered:

- **Natural-language / directive-driven creation** (ChatGPT-style "type what you want"): excellent mobile fit — one small typed directive replaces N fields; directly matches the app's existing LLM extraction pipeline and `data.user_directive` channel. *(Primary — adopted.)*
- **Progressive disclosure / chunked single-field "type what you know"** (NNGroup progressive-disclosure guidance; chunked single-question forms à la monotask forms): good mobile fit — one field at a time, fields appear only when needed. *(Adopted as the confirm/repair step and hint prompts.)*
- **Conversational / AI-assisted forms** (one question at a time with suggestions): strong fit but heavier to build; adopted partially as the directive-hint prompts.
- **Unified composer + attachment strip** (messaging-app pattern: one input area with persistent camera/file/paste icon strip): adopted — this is what unifies item 5 and item 6.
- **Bottom-sheet action sheets** (mobile standard for a [+] menu): adopted as the [+] on mobile; wide screens may use the same composer inline.
- **Drag-and-drop-first dropzones**: **rejected as primary** (mobile-first PWA — true DnD is a desktop idiom); retained only as a secondary desktop drop-on-composer capability.

Adopted pattern: **directive-first unified composer with progressive disclosure** — a single creation surface per context whose primary input is one flexible text field ("describe / type what you know"), with an attach strip for files/images, followed by an AI-prepared, progressively-disclosed **review & repair** step (fields revealed only as chips the user confirms/tweaks), not a blank multi-field form.

## ADDED Requirements

### Requirement: Creation flows use directive-first composers, not forms

Every add/create flow in the app (asset, account, review, document, and the unified landing ingest) SHALL follow the **directive-first composer** pattern:

1. The surface opens with a **single primary input** — placeholder in the style "Describe it — brand, model, anything you remember" — plus (where a document/image is expected) an **attach strip** of icon actions (camera / pick file / paste text).
2. An **optional directive note** field is always available ("Anything the scanner should know?") which is carried as the ingestion user directive per `document-ingestion` (data.user_directive).
3. On submit, the existing extraction pipeline runs and the user is shown a **review & repair step** with progressive disclosure: extracted fields render as a short summary of chips (brand, model, serial, price…) the user can confirm, tap to reveal, or correct; unextracted optional fields do NOT render as an endless blank-form grid.
4. No add flow SHALL open with a multi-field form as its first interaction, and reusing legacy form components in the new flows SHALL NOT reintroduce field-by-field entry as the primary interaction.

Backward compatibility: existing flow data shapes, api endpoints, and jsonb payload storage are unchanged — this requirement constrains the interaction pattern only; any backend support needed (e.g. surfacing extraction results for the review step) reuses the existing extraction outputs.

#### Scenario: Asset creation starts with one input

- **WHEN** the user opens the asset creation composer
- **THEN** the first and most prominent element is a single text input with a describe-style placeholder and an optional directive note field — not brand/model/serial/purchase-date empty fields laid out as a form

#### Scenario: Chips instead of a blank form

- **WHEN** the composer submits (word-problem paste: "MacBook Air 2019, serial unknown, paid 39999.99 in rupees") and extraction returns fields
- **THEN** the review step shows the extracted values as confirmable chips (brand Apple, model MacBook Air, price 39999.99…) and the user can tap a chip to reveal/correct it; fields extraction did not find stay collapsed behind a "add details" affordance, not pre-rendered as empty boxes

#### Scenario: User may skip review

- **WHEN** the user taps save/confirm immediately on the review step without editing any chip
- **THEN** the entity is created with the extracted values and the user lands on the success state (spec does not require filling any field)

#### Scenario: Directive note always available

- **WHEN** the composer is open
- **THEN** the optional directive note field is present and its contents are passed through the ingestion directive channel (data.user_directive) identical to the document upload note

#### Scenario: Old-style form is gone from the add flow

- **WHEN** the add/creation flows are exercised end-to-end (asset, account, review, document)
- **THEN** none of them is reachable via the legacy field-by-field form UI; the prior form implementations are removed or demoted to non-primary affordances
