# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the versioning [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- People list: sort by surname, birth (oldest or youngest first) or recent changes, and filter by
  sex and living or deceased, kept in the address; the sources list and the media gallery load more
  with a "Show more" button instead of stopping at 100 sources and 200 media (#60)

- The relationship calculator also names in-laws through two marriages and step-siblings (#43)

- Error messages from the server are shown in the interface language (#24)

- Kazakh relationship names (to be proofread by a native speaker) (#24)

- Russian relationship names (#24): the relationship page names kin in correct Russian with gender,
  half-blood (единокровный/единоутробный), step and in-law terms (свёкор, тёща, деверь, золовка,
  шурин, невестка…) by the spouse's side, and cousins of any degree (двоюродный, троюродный…)

- Calendar feeds in Russian or Kazakh (#24)

- Dates typed in Russian or Kazakh are understood (#24)

- Russian translation of the interface (#24): all screens, messages and plurals are in Russian;
  phrases that would need declined names ("X's birth") are worded with a colon or brackets instead

- Kazakh translation of the interface (to be proofread by a native speaker) (#24)

- Language switcher (Russian and Kazakh) next to the theme switch: the choice is saved in the
  browser, the first visit follows the browser language and falls back to Russian, the page
  language and dates follow it; the translations themselves come in later changes (#24)

- Migration map: when a GEDCOM import brought no coordinates, the map says so, lists the places
  used by dated events that lack them and lets you enter latitude and longitude right there, or
  look a place up on OpenStreetMap (Nominatim) with one click and pick a match (#12)

- Statistics: lifespan by decade of birth, age at first and later marriages, children per family,
  the most common surnames and given names and how given names changed by decade, each chart with
  its numbers; migration map: watch the family move through their dated events on an
  OpenStreetMap map with a year slider and play button, for everyone or the ancestors or
  descendants of a person, with a table of who was where in the chosen year (#38)

- Heirlooms: record objects handed down in the family with what they are, when and where they were
  made and where they are now, a timeline of who held them and how they got them, photos and
  sources; each person page lists the heirlooms they held, and GEDCOM files carry them as
  documented _HEIRLOOM records that GoTree reads back (#36)

- Wall chart: ancestors, descendants or both as a print-ready chart on A4 to A0, Letter, Tabloid
  or a custom size, fitted to the page with a warning when names get too small, with photos and a
  title; print it or save it as PDF from the browser, or download a standalone SVG (#34)

- Source-first transcription: copy a census household, baptism, marriage, burial, death or any
  other record into a grid, or define your own record templates; each person is matched against
  the tree, also across spelling variants (Meyer, Maier, Mayr), and adding the record creates the
  new people, the event with everyone's role, the couple's or parents' family, occupations,
  residences and births estimated from ages, each cited to its line in the record and to the value
  it supports (#32)

- Research tasks and a research log: plan what to find out (status, priority, due date, linked
  people, sources and places) and record every search with where and when, including the ones that
  found nothing; a missing or unsourced birth, a missing death or any data-quality problem becomes
  a task with one click; open tasks show on the home page, and each person page lists their tasks,
  searches and suggestions (#30)

- Calendar of birthdays, wedding anniversaries and remembrance days to subscribe to in any
  calendar app, through a secret address that can be withdrawn; living people only when chosen
  (#25)

- Share links: the owner creates read-only links for relatives without an account, for the whole
  tree or one branch, leaving living people out or showing them by name only, optionally until a
  date; visitors browse people and the tree, and a link can be withdrawn at any time. “On this
  day” lists births, marriages and deaths of deceased relatives on today's date, on the home page
  and in the shared view (#24)

- Backup: administrators download the database (a consistent snapshot taken while GoTree runs)
  and all photos and documents as one zip, with restore instructions inside; light, dark or
  system theme, chosen in the header and remembered per browser (#23)

- Data quality: GoTree checks the tree for dates that contradict each other (born after death,
  parents too young or too old, events after a burial, marriages outside a lifetime) and for dates
  it cannot read, on a data quality page and on each person's page; vague dates only count when
  every reading of them is impossible. “Tidy up dates” rewrites dates such as “12.3.1850”,
  “ca. 1850” or “March 12, 1850” in standard GEDCOM form after a preview where each row can be
  left out. The relationship calculator names how two people are related (cousins with removals,
  half and step relations, in-laws, pedigree collapse) and shows the line connecting them (#21)

- GEDCOM export as 5.5.1 or 7.0, optionally zipped with all photos and documents, with everyone,
  living people by name only, or without living people; everything an import could not use is
  written back, and “Check the export” reads the file back to show that nothing was lost (#19)

- GEDCOM import (5.5, 5.5.1 and 7.0, also zipped) from Ancestry, MyHeritage, FamilySearch,
  webtrees, Gramps or RootsMagic, in UTF-8, UTF-16, ANSI or ANSEL; a report shows what was imported,
  kept unchanged for the export, or dropped, and lists dates and references that need attention
  (#17)

- Photos, documents and recordings: upload them to people (stored once even when uploaded twice),
  view, describe and transcribe them, tag faces by drawing a box or import the face tags other
  photo tools wrote, and use a photo or a face as a person's portrait, shown in lists and on the
  tree (#15)

### Fixed

- Share links that show living people by name only no longer find them by nickname or other names,
  such as a maiden name; only their given names and surname are searched, and such links list
  matches in name order, since relevance also weighed the hidden names (#58)

- GEDCOM export and import no longer lose data on a round trip (#51): heirloom custody dates
  are written as GEDCOM dates (`12 MAR 1850` instead of `12.3.1850`), disputed and disproven
  events and names keep their status and reason, GEDCOM 7 participant roles (head, sibling,
  relative, informant and custom roles) and religious names come back as written, and an
  interpreted date with a dual year (`INT 1750/51`) keeps its second year in GEDCOM 7

- Searching places, sources, repositories and heirlooms ignores the case of Russian and Kazakh
  letters and treats ё as е, so «москва» finds «Москва» (#52)

- Dates in Kazakh no longer repeat «ж.» and no longer fall back to English in browsers without Kazakh
  locale data, and Russian months after prepositions are declined («до февраля 1820 г.») (#50)

- GEDCOM import no longer loses family links: a pointer followed by spaces (`1 FAMC @F1@ `) and a
  record line with several spaces after its identifier are read, and a child that names its family
  only in its own record (INDI.FAMC, no FAM.CHIL) is added to the family and listed in the import
  report (#53)

- The people list and a share link's people list no longer sort the whole tree for every page,
  so a share link on a large tree answers quickly, and paging through search results no longer
  repeats or skips people whose names differ only in case or accents (#49)

- Kazakh and Russian wording proofread (#46)

- Errors in transcription rows and in GEDCOM files are shown in the interface language (#43)

- «й» and the Kazakh letters sort in alphabet order, and transcriptions reuse a place whatever the
  case of its Cyrillic name (#43)

- Russian and Kazakh names sort in alphabetical order and match their Latin spellings when looking
  for duplicates, and searching «петр» finds «Пётр» (#24)

- A GEDCOM file inside a zip that unpacks to more than four times the upload limit is refused
  (413) instead of being silently cut off and, with "Replace the tree", imported truncated after
  wiping the tree, and a small zip can no longer use up to 1 GB of memory; replacing the tree by
  import is now limited to its owner, and the Import page tells editors so instead of offering it (#6)

- Importing a GEDCOM 7 file no longer loses the date text that GoTree's own 7.0 export keeps in
  DATE.PHRASE: dates that are not dates, the explanation of "INT" dates and the second year of
  dual dates such as 1750/51 come back as they were (#4)

- A data directory whose path contains `#` or `?` no longer opens the wrong database file (#19)

- GEDCOM export that excludes living people or reduces them to their name no longer includes the
  marriage and other events of a family with a living partner, nor the photos and other media
  (portraits and face tags included) that belong only to those living people (#3)

- The server closes idle keep-alive connections and drops uploads and imports that trickle in
  too slowly (answered with a timeout message); on shutdown running requests get 30 seconds to
  finish, then they are cancelled and given a few more seconds before the database is closed, and
  a second Ctrl+C stops GoTree at once (#7)

- Images over 50 megapixels are refused on upload with a clear error and get no thumbnail, so a
  small file declaring huge dimensions can no longer exhaust memory; at most two thumbnails
  are rendered at a time (#5)

- A place name containing HTML, typed or imported from a GEDCOM file, no longer runs script in the
  migration map's tooltips, and the app now sends a Content-Security-Policy that only allows
  scripts from its own files (#2)

### Changed

- The export check is a POST request, only editors and owners can run it, and only one runs at a
  time; a second one is asked to wait (#58)

- Dates read as plain English ("26 February 1820", "about 1850", "between 1850 and 1860") on the
  timeline, heirlooms and in the date field preview, and can be typed that way in events,
  heirlooms, media and transcriptions (saved as GEDCOM when needed, so the preview matches what is
  stored); "Tidy up dates" no longer offers dates that were just saved; share, expiry and due
  days now follow the interface language everywhere (#14)

- Navigation: the sidebar groups its sections under Tree, Records, Explore, Work and Settings
  and now lists the wall chart; the phone bar keeps Home, People, Tree and Sources and a "More"
  menu opens every section, grouped the same way (replacing the "Also" links on the home page);
  person pages link to "View in tree"; the relationship calculator, migration map and "Ancestors
  and descendants" chart use one name in menus, headings and default chart titles (#13)

- Home page: an empty tree now offers "Add yourself" and "Import from Ancestry, MyHeritage and
  others" (also on the empty Tree and People pages); a filled tree shows how many people,
  families and sources it holds, and "On this day" appears only when it has entries; the "More"
  block is gone and the server version is shown to administrators only (#9)

- The new-person and add-relative forms take an optional birth and death date and place, so
  "born 1850" no longer needs a second dialog; the events are saved together with the person (#8)

- Closing a form with Escape, the × button or Cancel after typing in it now asks "Discard changes?"
  instead of throwing the input away; forms you have not changed still close at once, and saving
  never asks (#10)

- A new version of GoTree no longer reloads open pages by itself; a notice offers to reload, so
  nothing being typed or uploaded is lost (#15)

- Tree page on phones: the controls collapse behind an "Options" button so the chart is visible
  at once, the selected person's actions open as a bottom sheet over the chart (Escape closes
  it), zoom buttons are 44px, names open at a readable zoom, and double-click no longer zooms
  the chart (use "Center tree here" in the sheet); the minimap is themed, smaller and shown on
  wide screens only (#11)

- Sources and archives: record where facts come from, cite a source with page and reliability
  on events, people and relationships, and see on each source what it supports (#13)

- Family tree charts: ancestors (pedigree), descendants, both (hourglass) and the family group,
  with pan and zoom, a generations slider and a "blood relatives only" filter; select a person to
  center the tree on them, open their page or add relatives (#11)
- Arrow keys move between related people in the chart, and every chart is also available as a
  list for screen readers and small screens (#11)

- People list with search, and a person page with life events in date order, parents and
  siblings, partners and children (#9)
- Add parents, partners, children and siblings in any order, as new or existing people; adding
  a married partner creates the marriage event (#9)
- Dialogs to edit people, events (with a live date preview, place search and creation, shared
  participants and fact status) and families; keyboard shortcuts, `?` lists them (#9)

- People with alternate names, families with unknown partners and per-parent child relations,
  events with GEDCOM dates, shared events with roles, a place hierarchy, and disputed or
  disproven facts; name search ignores accents and covers alternate names (#7)
- First-run setup, login and logout; every change is recorded in a change log (#7)

- Single-binary server with embedded web UI, SQLite storage with automatic migrations, a
  health endpoint and a Docker image; the responsive, accessible app shell installs as a
  web app (#5)

- Release script and workflow: `release.sh` cuts a tagged release from the commit history, and
  the release workflow publishes binaries and a Docker image for every tag (#1)
