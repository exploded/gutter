package main

import (
	"sort"
	"strings"
)

// Suburb is one entry in the published service area. Each gets a local landing
// page at /areas/{slug}; the copy is authored here so every page says something
// specific about the place rather than swapping the name into a template.
type Suburb struct {
	Name     string
	Slug     string
	Postcode string
	LGA      string // council area, used to group the index and pick "nearby" links
	DriveMin int    // rough drive time from Warrandyte, minutes
	Blurb    string // 2-3 sentences of local context shown on the page
	MetaDesc string // meta description (unique per suburb)
	Photo    *Photo // credit for static/img/areas/{slug}.jpg when it needs one (nil = own photo / no photo)
}

// Photo is the attribution for a suburb page image sourced under a free licence.
// Own photos need no Photo entry — just drop the file in (see tools/areaphoto).
type Photo struct {
	Artist     string // photographer / uploader as credited at the source
	Licence    string // "CC BY-SA 4.0", "Public domain", ...
	LicenceURL string // deed URL ("" for public domain)
	Source     string // where it came from (Commons file page)
}

// URL returns the canonical path for the suburb page.
func (s *Suburb) URL() string { return "/areas/" + s.Slug }

// suburbGroup is one council area on the /areas index.
type suburbGroup struct {
	LGA     string
	Suburbs []*Suburb
}

// lgaOrder controls the order of council groups on /areas (closest first).
var lgaOrder = []string{"Manningham", "Nillumbik", "Maroondah"}

var (
	suburbs       []string // display names, in catalogue order (used by templates + JSON-LD areaServed)
	suburbsBySlug = map[string]*Suburb{}
)

func init() {
	for i := range suburbList {
		suburbsBySlug[suburbList[i].Slug] = &suburbList[i]
		suburbs = append(suburbs, suburbList[i].Name)
	}
}

// retiredSuburbs are slugs that once had a page but were dropped from the
// service area. They 301 to /areas so indexed URLs don't become 404s. Empty
// for now: this is a new site, so nothing has been retired yet. Add a slug
// here whenever one is removed from suburbList.
var retiredSuburbs = map[string]bool{}

func findSuburb(slug string) (*Suburb, bool) {
	s, ok := suburbsBySlug[slug]
	return s, ok
}

// findSuburbByName matches a display name case-insensitively (used to validate ?suburb= prefills).
func findSuburbByName(name string) (*Suburb, bool) {
	name = strings.TrimSpace(name)
	for i := range suburbList {
		if strings.EqualFold(suburbList[i].Name, name) {
			return &suburbList[i], true
		}
	}
	return nil, false
}

// Nearby returns up to six other suburbs, same council first, then by drive time.
func (s *Suburb) Nearby() []*Suburb {
	var out []*Suburb
	for i := range suburbList {
		if suburbList[i].Slug != s.Slug {
			out = append(out, &suburbList[i])
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ai, aj := out[i].LGA == s.LGA, out[j].LGA == s.LGA
		if ai != aj {
			return ai
		}
		return absInt(out[i].DriveMin-s.DriveMin) < absInt(out[j].DriveMin-s.DriveMin)
	})
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// groupSuburbs buckets the catalogue by council area for the /areas index.
func groupSuburbs() []suburbGroup {
	byLGA := map[string]*suburbGroup{}
	var groups []*suburbGroup
	for _, l := range lgaOrder {
		g := &suburbGroup{LGA: l}
		byLGA[l] = g
		groups = append(groups, g)
	}
	for i := range suburbList {
		g, ok := byLGA[suburbList[i].LGA]
		if !ok {
			g = &suburbGroup{LGA: suburbList[i].LGA}
			byLGA[suburbList[i].LGA] = g
			groups = append(groups, g)
		}
		g.Suburbs = append(g.Suburbs, &suburbList[i])
	}
	var out []suburbGroup
	for _, g := range groups {
		if len(g.Suburbs) > 0 {
			out = append(out, *g)
		}
	}
	return out
}

// suburbList is the published service area (roughly 20 minutes' drive from
// Warrandyte). Postcodes, councils and local facts are authored by hand and
// were checked against council, CFA and Wikipedia sources in October 2026 —
// keep them accurate. Bushfire overlay claims are deliberately hedged ("parts
// of", "large parts of") because the overlay is mapped block by block.
var suburbList = []Suburb{
	// ── Manningham ──
	{Name: "Warrandyte", Slug: "warrandyte", Postcode: "3113", LGA: "Manningham", DriveMin: 5,
		Blurb:    "Warrandyte is home base. Houses here sit on bush blocks along the Yarra, where gums drop leaves, bark and gumnuts all year and the deciduous trees along the main streets add a big autumn load. Large parts of the township are in the Bushfire Management Overlay, and the local CFA brigade says plainly that all residents are at risk, so we book Warrandyte cleans before the Fire Danger Period starts, not after.",
		MetaDesc: "Gutter cleaning in Warrandyte 3113 from a local business. Fixed prices from $219, debris taken away, photo report. Book online.",
		Photo:    &Photo{Artist: "Nick carson at English Wikipedia", Licence: "Public domain", LicenceURL: "", Source: "https://commons.wikimedia.org/wiki/File:Yarra_River_at_Warrandyte.jpg"}},
	{Name: "Warrandyte South", Slug: "warrandyte-south", Postcode: "3134", LGA: "Manningham", DriveMin: 8,
		Blurb:    "Warrandyte South is the small, leafy pocket between Ringwood–Warrandyte Road, Anzac Road, Jumping Creek and Old Warrandyte Road, inside Manningham's green wedge. Most of the green wedge is designated bushfire prone, and the bush blocks here put gum leaves and bark into gutters all year. A clean in early spring and another in May after the autumn drop suits most homes here.",
		MetaDesc: "Gutter cleaning in Warrandyte South 3134 for bush blocks: gutters cleared with no one on a ladder, fixed prices. Book online."},
	{Name: "Park Orchards", Slug: "park-orchards", Postcode: "3114", LGA: "Manningham", DriveMin: 10,
		Blurb:    "Park Orchards started as a 1920s country club estate, and many of the pines planted during the Depression are still standing. Pine needles mat together in gutters and hold water, and with gums around The 100 Acres bushland and large parts of the suburb in the Bushfire Management Overlay, gutters here need regular attention. It's about ten minutes from our base in Warrandyte.",
		MetaDesc: "Pine needles and gum leaves cleared from gutters in Park Orchards 3114. Fixed prices from $219, mess taken away, photo report. Book online.",
		Photo:    &Photo{Artist: "Kiewa", Licence: "CC BY-SA 3.0", LicenceURL: "https://creativecommons.org/licenses/by-sa/3.0", Source: "https://commons.wikimedia.org/wiki/File:Park_Orchards_shops,_Park_Road,_Park_Orchards,_Australia.jpg"}},
	{Name: "Wonga Park", Slug: "wonga-park", Postcode: "3115", LGA: "Manningham", DriveMin: 10,
		Blurb:    "Wonga Park is low-density and semi-rural, stretching from Jumping Creek out to the Yarra, with big lots and plenty of canopy over the roofs. In late 2023 Manningham added about 600 more Wonga Park properties to the Bushfire Management Overlay, on top of the land already covered along the river. If yours was one of them, a gutter clean before the Fire Danger Period is an easy first step.",
		MetaDesc: "Gutter cleaning in Wonga Park 3115, in or near the Bushfire Management Overlay. Fixed prices, no ladders, dated photo report. Book online.",
		Photo:    &Photo{Artist: "Melburnian", Licence: "CC BY 2.5", LicenceURL: "https://creativecommons.org/licenses/by/2.5", Source: "https://commons.wikimedia.org/wiki/File:Yarra_River_Wonga_Park.jpg"}},
	{Name: "Donvale", Slug: "donvale", Postcode: "3111", LGA: "Manningham", DriveMin: 12,
		Blurb:    "Donvale runs from suburban streets near Doncaster Road out to green wedge blocks on the Warrandyte border, and parts of it are in the Bushfire Management Overlay. Bushland like Currawong Bush Park and the Mullum Mullum Creek corridor keeps plenty of eucalypts close to houses, and their leaves end up in gutters. Units through to large split-level homes are all on the fixed price list.",
		MetaDesc: "Gutter cleaning in Donvale 3111: fixed prices from $219, debris bagged and taken away. Book online.",
		Photo:    &Photo{Artist: "Nick carson at English Wikipedia", Licence: "Public domain", LicenceURL: "", Source: "https://commons.wikimedia.org/wiki/File:Mullum_mullum_creek_linear_park_path.JPG"}},
	{Name: "Templestowe", Slug: "templestowe", Postcode: "3106", LGA: "Manningham", DriveMin: 15,
		Blurb:    "Templestowe was orchards and green belt until the 1970s, and it still has big blocks, eucalypt gullies and Westerfolds Park along the Yarra. Mature gardens and street trees put a steady load into gutters, and parts of the suburb are in Manningham's green wedge. A two-storey house here is a fixed $489, with everything taken away.",
		MetaDesc: "Gutter cleaning in Templestowe 3106: $289 single storey, $489 double storey, no ladders, photo report. Book online.",
		Photo:    &Photo{Artist: "Ottre", Licence: "Public domain", LicenceURL: "", Source: "https://commons.wikimedia.org/wiki/File:Streetview_Templestowe.jpg"}},
	{Name: "Doncaster East", Slug: "doncaster-east", Postcode: "3109", LGA: "Manningham", DriveMin: 15,
		Blurb:    "Doncaster East sits in the hills between Koonung Creek and Mullum Mullum Creek, mostly 1960s to 1980s housing on old orchard land. Some of the pine windbreaks planted by the German settlers of Waldau in the 1860s are still standing, and pine needles mat into gutters and hold water. There are plenty of units here too, and those start at $219.",
		MetaDesc: "Gutter cleaning in Doncaster East 3109: units from $219, houses from $289, pine needles and leaves cleared, mess taken away. Book online.",
		Photo:    &Photo{Artist: "Bob Tan", Licence: "CC BY 4.0", LicenceURL: "https://creativecommons.org/licenses/by/4.0", Source: "https://commons.wikimedia.org/wiki/File:Aerial_panorama_of_Ruffey_Lake_Park._Sunset_24_September_2023.jpg"}},

	// ── Nillumbik ──
	{Name: "North Warrandyte", Slug: "north-warrandyte", Postcode: "3113", LGA: "Nillumbik", DriveMin: 5,
		Blurb:    "Across the bridge, North Warrandyte shares the 3113 postcode but sits in Nillumbik, on hilly ground with winding roads under a wide mix of eucalypts. Much of it is in Nillumbik's Bushfire Management Overlay, and in 2025 Nillumbik's Fire Danger Period started on 24 November, five weeks before Manningham's. If you're on this side of the river, book your spring clean early.",
		MetaDesc: "North Warrandyte gutter cleaning before the Fire Danger Period: fixed prices from $219, no ladders, debris taken away. Book online."},
	{Name: "Research", Slug: "research", Postcode: "3095", LGA: "Nillumbik", DriveMin: 8,
		Blurb:    "Research was Research Gully until late in the 1800s, named for a second search for gold. Today it's bush blocks of one to 35 acres under eucalypts and paperbarks, and parts of it fall within Nillumbik's Bushfire Management Overlay. Leaves and shedding bark from both end up in gutters and downpipes, so plan on two cleans a year here.",
		MetaDesc: "Gutter cleaning in Research 3095 for bush blocks under gums and paperbarks. Fixed prices, mess taken away, before-and-after photos. Book online."},
	{Name: "Kangaroo Ground", Slug: "kangaroo-ground", Postcode: "3097", LGA: "Nillumbik", DriveMin: 12,
		Blurb:    "Kangaroo Ground is farming country on the hills north of Warrandyte, and the memorial tower on Garden Hill is still used to spot fires each summer. Homes here often sit among paddocks, old shelter trees and remnant gums, so gutters catch gum leaves, bark and whatever the wind carries. The CFA sets the Fire Danger Period date each year, so book your clean in spring before it's declared.",
		MetaDesc: "Gutter cleaning in Kangaroo Ground 3097 before fire season. Fixed prices by house size, no ladders, debris taken away. Book online."},
	{Name: "Eltham", Slug: "eltham", Postcode: "3095", LGA: "Nillumbik", DriveMin: 15,
		Blurb:    "Eltham is known for mudbrick homes in the Alistair Knox tradition and the Montsalvat artists' colony, with green wedge land wrapped around the town. A lot of houses sit right under big eucalypts, so leaves, bark and twigs land in the gutters all year and pile up at the corners. It's about 15 minutes from Warrandyte through Research.",
		MetaDesc: "Gutter cleaning in Eltham 3095 under the big gums. Fixed prices from $219, mess taken away, photo report. Book online."},
	{Name: "Eltham North", Slug: "eltham-north", Postcode: "3095", LGA: "Nillumbik", DriveMin: 18,
		Blurb:    "Eltham North sits on the west side of Diamond Creek, with bushland reserves and the Diamond Creek Trail running through it, and it's split between Nillumbik and Banyule councils. Trees along the creek and in established gardens fill gutters faster than people expect. The first sign is usually overflow at the corners in heavy rain, which often means a blocked downpipe.",
		MetaDesc: "Eltham North gutter cleaning with fixed prices from $219: every gutter cleared with professional equipment, mess taken away. Book online."},

	// ── Maroondah ──
	{Name: "Ringwood North", Slug: "ringwood-north", Postcode: "3134", LGA: "Maroondah", DriveMin: 10,
		Blurb:    "Ringwood North is hilly, especially around Loughnan's Hill and Glenvale Road, and it sits on the Warrandyte side of Ringwood. A bushfire burnt between Warrandyte and Ringwood in January 1913, and parts of Maroondah are still designated bushfire prone. On sloping blocks the high side of the roof is hard to see from the ground, so blocked gutters there often go unnoticed until they overflow.",
		MetaDesc: "Gutter cleaning in Ringwood North 3134 on sloping blocks. Fixed prices from $219, no one on a ladder, dated photo report. Book online.",
		Photo:    &Photo{Artist: "Philip Mallis", Licence: "CC BY-SA 2.0", LicenceURL: "https://creativecommons.org/licenses/by-sa/2.0", Source: "https://commons.wikimedia.org/wiki/File:Maroondah_Highway,_North_Ringwood.jpg"}},
	{Name: "Warranwood", Slug: "warranwood", Postcode: "3134", LGA: "Maroondah", DriveMin: 12,
		Blurb:    "Warranwood takes its name from Warrandyte South and Ringwood North, and it was still mostly bushland in the early 1970s. Warranwood Reserve keeps 11 hectares of native bush along Jumping Creek, and plenty of homes nearby sit under tall trees. That means a steady fall of leaves and bark into gutters, so a spring clean before summer is the minimum we'd suggest.",
		MetaDesc: "Gutter cleaning in Warranwood 3134, close to Warrandyte. Fixed prices from $219, no ladders, debris taken away. Book online."},
	{Name: "Croydon Hills", Slug: "croydon-hills", Postcode: "3136", LGA: "Maroondah", DriveMin: 15,
		Blurb:    "Croydon Hills was farmland and orchards until it was developed in the 1980s, mostly as single-storey brick veneer homes on generous blocks. Gardens planted then have had 40 years to grow, and reserves like Candlebark Walk and Narr-Maen Reserve run between the streets. A single-storey home is a fixed $289, with no one on a ladder and everything taken away.",
		MetaDesc: "Croydon Hills gutter cleaning: $289 for a single-storey home, no ladders, debris taken away, with a photo report. Book online."},
}
