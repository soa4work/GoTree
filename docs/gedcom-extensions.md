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
holder is not in the file (not in the tree, or a living person left out for privacy); a `NOTE` then
says who it was. `TYPE` is how they got it: `inherited`, `gift`, `purchased`, `made`, `found` or
`other`.

## _from

`_HEIRLOOM._CUST._FROM` — since when the holder had the object, as a GEDCOM date (`12 MAR 1850`,
`ABT 1870`; in a 7.0 file with 7.0 keywords such as `JULIAN` and `BCE`). Text that is not a date is
written in parentheses: `(spring 1850)`. A dual year or an interpreted date is written as in 5.5.1
(`1750/51`) because a 7.0 date value cannot hold it.

## _to

`_HEIRLOOM._CUST._TO` — until when, in the same form.
