package main

import "html/template"

// Guide is a self-help article under /guides. Content is authored here (not
// user input) so Steps, IfNot and Before may contain a little inline HTML
// (<strong>, <em>, <a>). Summary and StopWhen are plain text — the templates
// escape them — so keep links out of those two.
type Guide struct {
	Slug     string
	Title    string
	Kicker   string // short category label; guides with the same Kicker link to each other
	Level    string // "Easy" (from the ground) | "Take care" (ladder or roof involved)
	Time     string // "15 minutes"
	Summary  string // card blurb + intro
	Before   string // optional caution shown before the steps
	Steps    []template.HTML
	IfNot    []template.HTML // "If that didn't help" bullets
	StopWhen string          // when to stop and book instead
	Service  string          // related service slug
	MetaDesc string
}

var guidesBySlug = map[string]*Guide{}

func init() {
	for i := range guides {
		guidesBySlug[guides[i].Slug] = &guides[i]
	}
}

func findGuide(slug string) (*Guide, bool) {
	g, ok := guidesBySlug[slug]
	return g, ok
}

// RelatedService resolves the related service (may be nil).
func (g *Guide) RelatedService() *Service {
	s, _ := findService(g.Service)
	return s
}

// guides is the catalogue, in display order. The index groups them by Kicker
// in order of first appearance, so the first guide's Kicker leads the page.
//
// Facts here are sourced from the CFA, WorkSafe Victoria, VicSES, the Building
// and Plumbing Commission (BPC, the VBA's successor from 1 July 2025) and
// Manningham and Nillumbik councils, checked October 2026. Fire Danger Period
// dates change every year: quote past dates as examples, never promise one.
var guides = []Guide{
	{
		Slug:    "fire-ready-gutters",
		Title:   "Getting your gutters fire-ready before the Fire Danger Period",
		Kicker:  "Bushfire",
		Level:   "Take care",
		Time:    "2–4 hours for a single-storey house",
		Summary: "Dry leaves in a gutter are exactly where embers land. This is the CFA's timing and checklist for gutters and roofs, plus what it means if the council sends you a Fire Prevention Notice.",
		Before:  `Clean gutters are one part of preparing a property, not the whole of it, and they don't make a house safe to shelter in. Have a <a href="https://www.cfa.vic.gov.au/fire-safety/your-fire-plan/make-your-fire-plan/bushfire-plan" target="_blank" rel="noopener">bushfire plan</a>: the CFA's advice is that leaving early is always the safest option. If you'll be on a ladder, read the <a href="/guides/cleaning-your-own-gutters-ladder-safety">ladder safety guide</a> first.`,
		Steps: []template.HTML{
			`<strong>Find out when your Fire Danger Period starts.</strong> The CFA declares it council by council, and the date moves every year. In 2025 it started on 24 November in Nillumbik and on 29 December in Manningham. From October, check the <a href="https://www.cfa.vic.gov.au/warnings-and-restrictions/fire-danger-period/fire-restriction-dates" target="_blank" rel="noopener">CFA fire restriction dates</a> for your council.`,
			`<strong>Clean in early spring, before that date.</strong> The CFA's <a href="https://www.cfa.vic.gov.au/plan-prepare/how-to-prepare-your-property" target="_blank" rel="noopener">property preparation guide</a> says to clear gutters of leaves, twigs and rubbish before the Fire Danger Period, keep clearing them regularly through the season, and make sure they're clear before a day rated Extreme or Catastrophic.`,
			`<strong>Do the hidden places, not just the front gutter.</strong> The CFA notes that wherever leaves collect around a house is generally where the wind carries embers. On a roof that means the valleys, box gutters, the join where a verandah or pergola roof meets the house, and the back corners you can't see from the street.`,
			`<strong>Bag the debris and take it away.</strong> Don't tip it into garden beds or leave a pile against the house, where it's fuel waiting for an ember. Don't burn it either: once the Fire Danger Period starts, burning off in the open needs a permit.`,
			`<strong>Flush the downpipes.</strong> Run a hose into the gutter near each downpipe and check the water comes out at the bottom. A blocked downpipe keeps the gutter above it full of wet leaf litter that dries out in summer.`,
			`<strong>Check the roof line for gaps.</strong> Broken tiles, gaps under ridge capping and unscreened vents let embers into the roof space. The CFA and building regulator's <a href="https://www.cfa.vic.gov.au/articledocuments/550/A-guide-to-Retrofit-Your-Home-for-Better-Protection-from-a-Bushfire_2014.pdf.aspx" target="_blank" rel="noopener">retrofit guide</a> recommends sealing gaps over 3 mm and screening openings with metal mesh of 2 mm or less. Note what you find; fixing roofs is a job for a roofer or roof plumber.`,
			`<strong>Keep a dated record.</strong> Take before-and-after photos of the cleaned gutters with the date on them. It's useful if the council or your insurer asks.`,
		},
		IfNot: []template.HTML{
			`<strong>Got a Fire Prevention Notice?</strong> Councils inspect properties for fire hazards in the lead-up to and during the Fire Danger Period. Manningham runs a <a href="https://www.manningham.vic.gov.au/news/fire-hazard-inspection-program-fhip" target="_blank" rel="noopener">Fire Hazard Inspection Program</a>, and Nillumbik's 2024 program covered more than 9,000 properties in its Bushfire Management Overlay. A notice lists the works you must do and the date they're due. In Manningham, ignoring one can mean a fine of up to 10 penalty units, plus the cost of a contractor the council sends to do the work. If anything in your notice is unclear, ring the council before the deadline.`,
			`Not sure whether your block is in the Bushfire Management Overlay or a designated bushfire prone area? Look up your address on <a href="https://mapshare.vic.gov.au/vicplan/" target="_blank" rel="noopener">VicPlan</a>.`,
			`Gutters full again a few weeks after the spring clean? Under heavy gums that's normal, and it's why the CFA says to keep clearing them through the season. The <a href="/services/fire-ready-plan">Fire-ready plan</a> books a second clean each May.`,
		},
		StopWhen: "The house is two storeys, the roof is steep or slippery, you'd be on the ladder alone, or a notice deadline is close. Book a fire-season clean and you get a dated before-and-after photo report with the receipt.",
		Service:  "bushfire-preparation",
		MetaDesc: "A CFA-aligned checklist for getting gutters fire-ready before the Fire Danger Period, and what a council Fire Prevention Notice means for you.",
	},
	{
		Slug:    "how-often-to-clean-gutters",
		Title:   "How often should you clean your gutters?",
		Kicker:  "Maintenance",
		Level:   "Easy",
		Time:    "10 minutes",
		Summary: "Under gum trees, once a year usually isn't enough. Here's how to work out the right rhythm for your roof without guessing.",
		Steps: []template.HTML{
			`<strong>Start from two cleans a year.</strong> For most homes near bush that means one in early spring, before the Fire Danger Period, and one in May after the autumn leaf drop. Spring is the CFA's timing; May catches the oaks, elms and other deciduous trees in the older streets once they've finished.`,
			`<strong>Look at what's overhead.</strong> Eucalypts are evergreen, but they drop leaves, bark and gumnuts all year and more in hot, dry spells. Smooth-barked gums like manna gum and candlebark shed long ribbons of bark that lie across a gutter and catch everything else. Pines drop needles that mat together and hold water.`,
			`<strong>Check how far the canopy reaches.</strong> If branches hang over the roof, plan on at least two cleans a year. If the nearest big tree is across the road, one clean a year may be enough. Check again after autumn to be sure.`,
			`<strong>Watch the first heavy rain after autumn.</strong> From inside or under cover, look for water spilling over the front edge of the gutter, especially at corners and next to downpipes. If it overflows, it's due.`,
			`<strong>Add a check before extreme fire days and storms.</strong> The CFA says to make sure gutters are clear before a day rated Extreme or Catastrophic. VicSES says to keep gutters, downpipes and drains clear before storms.`,
		},
		IfNot: []template.HTML{
			`Overflowing within a couple of months of a clean? It's often the downpipe rather than the gutter. See <a href="/guides/blocked-downpipe-check-and-flush">checking a blocked downpipe</a>.`,
			`Grass or seedlings growing out of the gutter means soil has built up from rotted leaves. That gutter is well past due.`,
			`Not sure what you can see from the ground? Try the <a href="/guides/signs-gutters-are-blocked">signs your gutters are blocked</a> walk-round.`,
		},
		StopWhen: "The canopy hangs over the roof, the house is two storeys, or you'd rather not have to remember. The Fire-ready plan books a clean each spring and each May at 12% off, and we send the reminder.",
		Service:  "fire-ready-plan",
		MetaDesc: "How often to clean gutters under gum trees in Melbourne's east and north-east: twice a year for most homes, in spring and May, plus checks before fire days and storms.",
	},
	{
		Slug:    "signs-gutters-are-blocked",
		Title:   "Signs your gutters or downpipes are blocked (checked from the ground)",
		Kicker:  "Maintenance",
		Level:   "Easy",
		Time:    "15 minutes",
		Summary: "You don't need a ladder to tell whether gutters need doing. Walk around the house with these checks, ideally during or just after rain.",
		Before:  `Stay on the ground for all of these. Keep well clear of the power line that runs to your house, and don't poke anything up towards it.`,
		Steps: []template.HTML{
			`<strong>Look along the gutter line from a distance.</strong> Stand back across the yard or the street. Leaves poking up above the gutter edge, grass or seedlings growing out of it, or a gutter that dips in the middle all mean it's full or sagging.`,
			`<strong>Watch it rain.</strong> Water pouring over the front edge instead of going down the downpipe is the clearest sign. Note which run or corner it is: that tells you which downpipe to check.`,
			`<strong>Listen to the downpipes.</strong> In rain, a working downpipe sounds like running water and flows out at the bottom. One that's quiet and dry while the gutter above it overflows is blocked. Water spurting from a joint means the blockage is below that joint.`,
			`<strong>Look at the fascia and eaves.</strong> Dark streaks, peeling paint or soft, rotting timber on the board behind the gutter mean water has been spilling backwards over the inside edge.`,
			`<strong>Check the ground under the gutter line.</strong> Channels washed into garden beds, mud splashed up the walls, or soggy soil along the base of the house all point to regular overflow.`,
			`<strong>Look at the ceilings inside.</strong> Water stains near outside walls after heavy rain can mean a gutter or box gutter is backing up into the roof. Don't leave that one long.`,
		},
		IfNot: []template.HTML{
			`Only one corner overflows and the rest looks fine? It's probably that downpipe. Try the <a href="/guides/blocked-downpipe-check-and-flush">downpipe check</a>.`,
			`Overflow everywhere in very heavy rain, even soon after a clean? See <a href="/guides/gutters-overflowing-in-rain">why gutters overflow</a>. It may be the gutter's size or fall, which is a plumber's job.`,
		},
		StopWhen: "You've spotted one or more of these signs and the roof isn't one you can safely get to. Book a clean and you'll get before-and-after photos of every run, so you can see what was up there.",
		Service:  "gutter-cleaning",
		MetaDesc: "Simple checks from the ground that tell you whether your gutters or downpipes are blocked, with no ladder needed. For homes in Melbourne's east and north-east.",
	},
	{
		Slug:    "cleaning-your-own-gutters-ladder-safety",
		Title:   "Is it safe to clean your own gutters? Ladder safety basics",
		Kicker:  "Maintenance",
		Level:   "Take care",
		Time:    "2–4 hours for a single-storey house",
		Summary: "Plenty of people do their own. But a single-storey gutter is already above 2 metres, the height above which WorkSafe Victoria says the risk of serious injury or death increases. If you're going up, set up the way WorkSafe expects tradies to.",
		Before:  `Don't do this one if you're unsteady on a ladder, home alone, or the house is two storeys. Never work at the roof edge on your own, even on a single-storey house, and never go up on a two-storey roof without proper fall protection.`,
		Steps: []template.HTML{
			`<strong>Use the right ladder.</strong> WorkSafe Victoria's <a href="https://www.worksafe.vic.gov.au/using-portable-ladders-workplace" target="_blank" rel="noopener">guidance on portable ladders</a> says workplace ladders should be industrial-grade with a safe working load of at least 120 kg. Check the feet, rungs and locks before you climb.`,
			`<strong>Stand it on firm, level ground at 4 to 1.</strong> For every 4 metres of height, the base sits 1 metre out from the wall. Not in a garden bed, on loose gravel, or on a slope you haven't levelled properly.`,
			`<strong>Extend it past the step-off point.</strong> If you'll step from the ladder onto the roof, it needs to reach 1 metre (about 3 rungs) above the gutter, and should be secured so it can't slip. Never stand on a rung less than 1 metre from the top.`,
			`<strong>Keep three points of contact.</strong> Two feet and one hand on the ladder at all times. That means a scoop in one hand and the bucket hooked to the ladder, not both hands busy.`,
			`<strong>Don't overreach.</strong> Keep your body between the rails. Climb down and move the ladder every metre or so. It's slower, and that's the point.`,
			`<strong>Look up for power lines.</strong> The service line comes into the house near the roof. Keep well away from it, and WorkSafe says to use a non-conductive ladder, such as fibreglass, wherever there's an electrical hazard.`,
			`<strong>Have someone with you.</strong> Someone to foot the ladder, pass things up and call for help if needed.`,
			`<strong>Leave two storeys to people with fall protection.</strong> At that height you need a roof anchor and harness or edge protection, and the training to use it. Without that, don't go up.`,
			`<strong>Stay off wet, mossy or steep roofs.</strong> Wait for a dry, still day. Tiles can crack underfoot and cause leaks, so if you can reach the gutter from the ladder, stay on the ladder.`,
		},
		IfNot: []template.HTML{
			`WorkSafe's rules are written for workplaces, but they're a good standard at home too. Read the full <a href="https://www.worksafe.vic.gov.au/using-portable-ladders-workplace" target="_blank" rel="noopener">portable ladders guidance</a> before you buy or borrow a ladder.`,
			`Want to know whether the gutters need doing before anyone goes up? The <a href="/guides/signs-gutters-are-blocked">ground-level checks</a> answer that without a ladder.`,
		},
		StopWhen: "The house is two storeys, you'd be on your own, the ground is sloping or soft, or you're not completely steady on a ladder. That's what we're for.",
		Service:  "gutter-cleaning",
		MetaDesc: "Cleaning your own gutters? WorkSafe-based ladder basics: 4 to 1 angle, 1 m above the step-off, three points of contact, and when not to go up.",
	},
	{
		Slug:    "blocked-downpipe-check-and-flush",
		Title:   "Downpipe blocked? How to check and flush it from the ground",
		Kicker:  "Rain & downpipes",
		Level:   "Easy",
		Time:    "20 minutes per downpipe",
		Summary: "If one corner of the gutter overflows while the rest is fine, the downpipe below it is the usual suspect. You can test and clear a lot of it without leaving the ground.",
		Before:  `Stay on the ground. Don't use chemical drain cleaners: stormwater drains run untreated to local creeks and the Yarra.`,
		Steps: []template.HTML{
			`<strong>Watch the bottom in rain.</strong> Water flowing out of the downpipe outlet means it's working. A dry outlet while the gutter above it overflows means it's blocked.`,
			`<strong>Tap your way up.</strong> Tap along the downpipe from the bottom up with your knuckle or a screwdriver handle. A hollow sound means it's clear; a dull, solid sound shows where leaves are packed.`,
			`<strong>Clear what's at the bottom.</strong> If the downpipe runs into a grated pit, lift the grate and scoop out the leaves. If it feeds a rainwater tank, clean the leaf strainer on the tank inlet. A clogged strainer backs water up the downpipe.`,
			`<strong>Open the cleaning eye if there is one.</strong> Some downpipes have a screw-off cap or clip-on cover near the bottom. Put a bucket underneath first: a blocked pipe lets go of water and sludge in a rush. Pull out what you can reach with a gloved hand or a bent piece of wire.`,
			`<strong>Flush it.</strong> With the cleaning eye open, feed a garden hose up the downpipe and turn it on gently to loosen packed leaves, then let it drain. If your hose has a telescopic wand that reaches the gutter from the ground, run water into the gutter at the downpipe and watch it come out the bottom.`,
			`<strong>Work out whether it's above or below ground.</strong> If water runs freely out of the downpipe but backs up once it's connected to the pipe in the ground, the blockage is in the underground stormwater drain.`,
		},
		IfNot: []template.HTML{
			`The blockage is right at the top, where the gutter drains into the pipe? That needs someone at the gutter. Read the <a href="/guides/cleaning-your-own-gutters-ladder-safety">ladder safety guide</a> first, or book.`,
			`Water backs up from underground, or there's a soggy patch or sinkhole along the drain line? That's often tree roots or a broken stormwater pipe, and it's drainage plumbing. Call a licensed plumber; you can check their licence with the <a href="https://www.bpc.vic.gov.au/find-and-check-a-practitioner" target="_blank" rel="noopener">Building and Plumbing Commission</a>.`,
			`Downpipe cracked, split, or pulled away from the wall? Repairs are licensed plumbing work in Victoria. A plumber fixes it.`,
		},
		StopWhen: "The blockage is out of reach, the downpipe is packed solid, or you'd rather not be on a ladder. On a clean we'll point out any blocked downpipe we find and, where it can be reached, clear it with a jetter for $90, only with your OK.",
		Service:  "downpipe-unblocking",
		MetaDesc: "Downpipe blocked? How to check it from the ground, clear the outlet and the pit, and tell when the blockage is underground and needs a plumber.",
	},
	{
		Slug:    "gutters-overflowing-in-rain",
		Title:   "Gutters overflowing when it rains: causes, and what's DIY vs plumber",
		Kicker:  "Rain & downpipes",
		Level:   "Easy",
		Time:    "15 minutes in the rain",
		Summary: "Overflow usually has one of four causes: a full gutter, a blocked downpipe, a gutter that has lost its fall, or more roof water than the gutters and downpipes can carry. The first two are cleaning jobs. The last two are for a plumber.",
		Steps: []template.HTML{
			`<strong>Note where it overflows.</strong> Overflow along a whole run points to a full gutter. Overflow at one spot, such as a corner or the low point beside a downpipe, points to that downpipe.`,
			`<strong>Look for leaves and plants.</strong> If you can see debris above the gutter edge from the ground, or seedlings growing in it, it's a cleaning job.`,
			`<strong>Check the downpipe.</strong> Follow the <a href="/guides/blocked-downpipe-check-and-flush">downpipe guide</a>. A blocked downpipe makes a clean gutter overflow.`,
			`<strong>Sight along the gutter for sag.</strong> Gutters are set with a slight fall towards the downpipes. A dip in the middle, brackets pulling away, or water still sitting in the gutter a day after rain means it has lost its fall.`,
			`<strong>Watch the valleys.</strong> Where two roof slopes meet, water concentrates. In heavy rain it can shoot straight over the gutter at the bottom of a valley.`,
			`<strong>Notice when it happens.</strong> Some gutters have small slots along the front, so that in very heavy rain they spill outside rather than back into the eaves. A little water from those in a downpour is by design. If clean gutters overflow in every heavy storm, the gutter or downpipes may be too small for the roof, or there may be too few downpipes.`,
		},
		IfNot: []template.HTML{
			`<strong>Cleaning jobs (you or us):</strong> leaves in gutters, a downpipe blocked with leaves, a pit grate clogged with debris.`,
			`<strong>Licensed plumber:</strong> re-hanging a sagging gutter, replacing rusted sections or leaking joints, adding or upsizing downpipes, box gutter repairs, and anything in the underground stormwater drain. Gutters and downpipes are roof plumbing, which in Victoria must be done by a licensed or registered plumber. Check your plumber with the <a href="https://www.bpc.vic.gov.au/find-and-check-a-practitioner" target="_blank" rel="noopener">Building and Plumbing Commission</a>.`,
		},
		StopWhen: "It's leaves or a blocked downpipe and you can't safely reach. If we find sag, rust or undersized gutters during the clean, we'll photograph it so you can show a plumber.",
		Service:  "gutter-cleaning",
		MetaDesc: "Gutters overflowing in the rain? The four usual causes, how to tell which one you have, and what's a cleaning job versus licensed plumbing work.",
	},
	{
		Slug:    "after-a-storm-gutter-check",
		Title:   "After a storm: what to check on your gutters and roof",
		Kicker:  "Rain & downpipes",
		Level:   "Easy",
		Time:    "20 minutes",
		Summary: "A storm can drop a season's worth of leaves, bark and branches in one night, and a gutter that was fine last week can be packed. Here's a safe walk-round from the ground.",
		Before:  `Stay away from fallen power lines: always assume they're live and keep 8 to 10 metres away. In a life-threatening emergency call 000. For storm damage help, call VicSES on 132 500.`,
		Steps: []template.HTML{
			`<strong>Walk around the house from the ground.</strong> Look for branches on the roof or hanging in gutters, broken or shifted tiles, gutters bent or pulled away from the fascia, and downpipes knocked off the wall.`,
			`<strong>Don't go up while it's wet or windy.</strong> Roofs stay slippery for hours after rain, and more storms often follow. Wait for a dry, still day.`,
			`<strong>Check the ceilings inside.</strong> Look for water stains, drips or sagging plaster, especially near outside walls and under box gutters. Sagging plaster can be holding water, so keep people out from under it and call for help.`,
			`<strong>Clear the ground-level drains.</strong> Lift the grates on stormwater pits and scoop out leaves so the next downpour can get away.`,
			`<strong>Photograph damage before anything is moved.</strong> Your insurer will want to see it.`,
			`<strong>Get ready for the next one.</strong> <a href="https://www.ses.vic.gov.au/plan-and-stay-safe/emergencies/storm" target="_blank" rel="noopener">VicSES advises</a> keeping gutters, downpipes and drains clear of debris, trimming branches back from the house, and securing outdoor furniture and trampolines.`,
		},
		IfNot: []template.HTML{
			`Roof damaged and water coming in? Call VicSES on 132 500 for emergency help.`,
			`Gutter bent, split or torn off the fascia? That's a repair for a licensed roof plumber, not a clean.`,
			`Tree down or a branch hanging over the house? That's an arborist's job, not one for a ladder.`,
		},
		StopWhen: "The storm has filled the gutters with leaves and debris but there's no structural damage. We'll clear the gutters and send photos, so you can see the roof without going up.",
		Service:  "gutter-cleaning",
		MetaDesc: "After a storm: a safe, from-the-ground check of gutters, downpipes, roof and ceilings, with VicSES advice and when to call 132 500.",
	},
}

// guidesForService returns the guides whose related service is slug, in
// catalogue order. Service pages link these so every guide has an inbound link
// from a service page rather than only from the /guides index.
func guidesForService(slug string) []*Guide {
	var out []*Guide
	for i := range guides {
		if guides[i].Service == slug {
			out = append(out, &guides[i])
		}
	}
	return out
}
