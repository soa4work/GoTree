# GEDCOM extensions written by GoTree

GoTree writes standard GEDCOM 5.5.1 or 7.0. Where the standard has no way to say something, it uses
the extension tags below; a GEDCOM 7 export declares them in `HEAD.SCHMA` with links to this page.
Anything GoTree read from another program and did not understand is written back unchanged.

## _frel

`FAM.CHIL._FREL <relation>` — how the child relates to the first partner of the family (`HUSB`).
Values: `Natural`, `Adopted`, `Foster`, `Step`, `Surrogate`, `Sealing`, `Unknown`. The same tag is
written by Ancestry and Family Tree Maker. Used only when the relation is not the same for both
partners or has no `PEDI` value.

## _mrel

`FAM.CHIL._MREL <relation>` — the same for the second partner (`WIFE`).

## _milt

`INDI._MILT` — military service, an event like `OCCU` with `DATE`, `PLAC` and a description.

## _stat

`<fact>._STAT <status>` with an optional `NOTE` giving the reason — marks a fact (event, attribute
or name) as `disputed` or `disproven`. GoTree keeps such facts instead of deleting them; programs
that do not know the tag still see the fact itself. GoTree reads the status and the reason back on
import; a status on the first (main) name is not kept, since the main name has none.

## _heirloom

`0 @H1@ _HEIRLOOM` — an object handed down in the family, as a record of its own:

```
0 @H1@ _HEIRLOOM
1 NAME Pocket watch
1 TYPE jewellery
1 _DESC Silver, engraved J.W.
1 DATE ABT 1880
1 PLAC Leipzig, Sachsen, Deutschland
1 _LOC Paul's desk
1 NOTE Repaired in 1950.
1 _CUST @I12@
2 _FROM 1920
2 _TO 1965
2 TYPE inherited
1 _CUST
2 _FROM 1965
2 TYPE purchased
2 NOTE a dealer in Halle
1 SOUR @S3@
2 PAGE § 3
1 OBJE @O7@
```

`NAME` is the object, `TYPE` its kind (`jewellery`, `furniture`, `document`, `photo_album`, `tool`,
`textile`, `other`), `DATE` and `PLAC` when and where it was made, `NOTE` free notes, `SOUR` and
`OBJE` sources and photos as for any record.

## _desc

`_HEIRLOOM._DESC` — what the object looks like.

## _loc

`_HEIRLOOM._LOC` — where the object is now, as free text.

## _cust

`_HEIRLOOM._CUST [@I…@]` — one holder of the object, in order. The pointer is left out when the
holder is not in the tree; a `NOTE` then says who it was. A holder hidden by an export without
living people keeps only `TYPE`. A living holder exported by name only keeps the pointer and `TYPE`.
Dates and notes would tell when they lived, and a note often names them. `TYPE` is how they got it:
`inherited`, `gift`, `purchased`, `made`, `found` or `other`.

## _from

`_HEIRLOOM._CUST._FROM` — since when the holder had the object, as a GEDCOM date (`12 MAR 1850`,
`ABT 1870`; in a 7.0 file with 7.0 keywords such as `JULIAN` and `BCE`). Text that is not a date is
written in parentheses: `(spring 1850)`. A dual year or an interpreted date is written as in 5.5.1
(`1750/51`) because a 7.0 date value cannot hold it.

## _to

`_HEIRLOOM._CUST._TO` — until when, in the same form.

## Participants (ASSO)

This is a convention, not a tag. GoTree keeps the people who took part in an event (a godparent at a
baptism, the witnesses of a marriage) with a role, an optional name of their own for the role, and
a note.

**GEDCOM 7.0.** The `ASSO` sits on the event, as the standard says: `ASSO @I7@` with `ROLE`, an
optional `PHRASE` under it, and an optional `NOTE`. A role 7.0 knows keeps its tag (`ROLE GODP`) and
its own name goes in the `PHRASE` (`4 PHRASE Taufpate`). The role `other` is `ROLE OTHER` with the
name in the `PHRASE`. The roles 7.0 lacks (`head`, `sibling`, `relative`, `informant`) are
`ROLE OTHER` with the role in the `PHRASE`, followed by `: <name>` when there is a name of its own
(`4 PHRASE informant: Hebamme`).

**GEDCOM 5.5.1.** `ASSO` is allowed only on a person, so it is written on the person whose event
it is, with the event in the `RELA`:

```
1 ASSO @I7@
2 RELA <role>[: <name>] (<TAG>[ <family>][ <n>])
2 NOTE <note>
```

`<TAG>` is the event type. `<n>` is written only for the second and later event of the same type of
that person (or family), counted in the order the events are written: `witness (RESI 2)`. The
participants of a family event (`MARR`, `DIV`, ...) are written on the family's first partner, with
the family in the `RELA`: `witness (MARR F3)`, `officiant (MARR F3 2)`. When the family's events are
left out of an export (a partner is excluded or reduced to a name), so are their participants.

Examples: `godparent (BAPM)`, `other: Trauzeuge (BAPM)`, `witness (RESI 2)`.

GoTree reads both forms back. An `ASSO` it cannot place (no such event, `VOID`) is kept as it is. A
source under an `ASSO` is not kept.
