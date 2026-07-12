"""
Pure-function USA / non-USA / unknown classifier for free-text job locations.

Ported from backend/service/job/location/classifier.go to keep the same
classification semantics in this POC (deterministic, no LLM):

  - "Any USA wins" - a multi-part location ("New York / London") that names
    at least one USA region is kept as USA.
  - "Unknown stays unknown" - empty strings, plain "Remote", "Worldwide" etc.
    have no clear signal either way.
  - Whole-word matching - tokens are matched with word boundaries so
    "Russian River, CA" does not match the country "Russia".
  - Within one segment, a non-USA token wins over a USA token (mirrors the
    Go implementation's defensive precedence rule).
"""

import re

USA = "usa"
NON_USA = "non_usa"
UNKNOWN = "unknown"

# Separators ATSs commonly use to join multiple locations into one string.
# Commas are NOT a separator - "San Francisco, CA" is one part, not two.
_PART_SPLITTER = re.compile(r"\s*(?:/|;|\||&|\n|\r)\s*")

# A US state code immediately after a comma: ", CA", ", NY", etc.
_STATE_SUFFIX = re.compile(r",\s*([A-Za-z]{2})(?:[\s,)]|$)")

US_STATE_CODES = {
    "AL", "AK", "AZ", "AR", "CA", "CO", "CT", "DE", "FL", "GA", "HI", "ID",
    "IL", "IN", "IA", "KS", "KY", "LA", "ME", "MD", "MA", "MI", "MN", "MS",
    "MO", "MT", "NE", "NV", "NH", "NJ", "NM", "NY", "NC", "ND", "OH", "OK",
    "OR", "PA", "RI", "SC", "SD", "TN", "TX", "UT", "VT", "VA", "WA", "WV",
    "WI", "WY", "DC", "PR",
}

USA_TOKENS = [
    "usa", "u.s.a.", "u.s.a", "u.s.", "u.s", "us",
    "united states", "united states of america", "america", "americas",
    "north america",
    "remote us", "remote usa", "remote u.s.", "remote united states",
    "remote - us", "remote - usa", "remote - united states",
    "remote (us)", "remote (usa)", "remote (united states)",
    "us remote", "usa remote", "us-remote", "usa-remote",
    "nationwide", "anywhere in us", "anywhere in usa", "anywhere in the us",
    "anywhere in the united states",
    "alabama", "alaska", "arizona", "arkansas", "california", "colorado",
    "connecticut", "delaware", "florida", "georgia", "hawaii", "idaho",
    "illinois", "indiana", "iowa", "kansas", "kentucky", "louisiana",
    "maine", "maryland", "massachusetts", "michigan", "minnesota",
    "mississippi", "missouri", "montana", "nebraska", "nevada",
    "new hampshire", "new jersey", "new mexico", "new york state",
    "north carolina", "north dakota", "ohio", "oklahoma", "oregon",
    "pennsylvania", "rhode island", "south carolina", "south dakota",
    "tennessee", "texas", "utah", "vermont", "virginia", "washington state",
    "west virginia", "wisconsin", "wyoming",
    "district of columbia", "washington, d.c.", "washington d.c.",
    "puerto rico", "guam", "u.s. virgin islands", "us virgin islands",
    "american samoa", "northern mariana islands",
    "new york", "new york city", "nyc", "brooklyn", "manhattan",
    "san francisco", "sf bay area", "bay area", "silicon valley",
    "palo alto", "mountain view", "menlo park", "redwood city",
    "sunnyvale", "san jose", "oakland", "berkeley",
    "los angeles", "san diego", "sacramento", "fresno",
    "seattle", "bellevue", "redmond",
    "portland, or",
    "chicago", "evanston",
    "boston", "cambridge, ma", "somerville", "waltham",
    "atlanta",
    "miami", "tampa", "orlando", "jacksonville",
    "dallas", "fort worth", "houston", "austin", "san antonio", "el paso",
    "denver", "boulder",
    "phoenix", "tempe", "scottsdale",
    "minneapolis", "st. paul", "saint paul",
    "detroit", "ann arbor",
    "philadelphia", "pittsburgh",
    "baltimore", "annapolis",
    "raleigh", "durham", "charlotte", "chapel hill",
    "nashville", "memphis", "knoxville",
    "new orleans", "baton rouge",
    "kansas city", "st. louis", "saint louis",
    "cincinnati", "cleveland", "columbus", "indianapolis",
    "salt lake city", "provo",
    "las vegas", "reno",
    "honolulu",
    "anchorage",
]

NON_USA_TOKENS = [
    "emea", "apac", "apj", "anz", "latam", "latin america",
    "europe", "european union", "eu", "asia", "asia pacific",
    "middle east", "africa", "oceania", "caribbean",
    "global ex-us", "outside us", "outside the us", "non-us", "non us",
    "international", "worldwide ex-us",

    "canada", "canadian", "toronto", "montreal", "vancouver", "calgary",
    "ottawa", "edmonton", "winnipeg", "quebec", "ontario", "alberta",
    "british columbia", "nova scotia", "saskatchewan", "manitoba",
    "halifax", "victoria, bc",

    "united kingdom", "uk", "u.k.", "great britain", "britain",
    "england", "scotland", "wales", "northern ireland", "ireland",
    "london", "manchester", "edinburgh", "glasgow", "birmingham",
    "leeds", "liverpool", "bristol", "dublin", "cork", "belfast",
    "oxford", "cambridge, uk",

    "france", "paris", "lyon", "marseille", "toulouse", "bordeaux",
    "germany", "berlin", "munich", "hamburg", "frankfurt", "cologne",
    "stuttgart", "düsseldorf", "dusseldorf", "kaiserslautern", "mannheim",
    "netherlands", "holland", "amsterdam", "rotterdam", "the hague",
    "belgium", "brussels", "antwerp",
    "spain", "madrid", "barcelona", "valencia", "seville",
    "portugal", "lisbon", "porto",
    "italy", "rome", "milan", "milano", "turin", "naples", "florence",
    "switzerland", "zurich", "geneva", "bern", "basel",
    "austria", "vienna", "salzburg",
    "luxembourg",

    "sweden", "stockholm", "gothenburg", "malmö", "malmo",
    "norway", "oslo", "bergen",
    "denmark", "copenhagen", "aarhus",
    "finland", "helsinki",
    "iceland", "reykjavik",

    "poland", "warsaw", "krakow", "kraków", "gdańsk", "gdansk", "wrocław", "wroclaw",
    "czech republic", "czechia", "prague",
    "slovakia", "bratislava",
    "hungary", "budapest",
    "romania", "bucharest", "cluj",
    "bulgaria", "sofia",
    "greece", "athens", "thessaloniki",
    "turkey", "istanbul", "ankara",
    "ukraine", "kyiv", "kiev", "lviv",
    "russia", "moscow", "saint petersburg", "st petersburg",
    "serbia", "belgrade", "croatia", "zagreb", "slovenia", "ljubljana",
    "estonia", "tallinn", "latvia", "riga", "lithuania", "vilnius",

    "india", "indian", "bangalore", "bengaluru", "mumbai", "delhi",
    "new delhi", "hyderabad", "chennai", "pune", "kolkata", "gurgaon",
    "gurugram", "noida", "ahmedabad", "indore", "jaipur", "bhubaneswar",
    "coimbatore", "nagpur", "kochi", "lucknow", "chandigarh", "surat",
    "visakhapatnam", "vizag", "thiruvananthapuram", "trivandrum",
    "china", "chinese", "beijing", "shanghai", "shenzhen", "guangzhou",
    "hangzhou", "chengdu",
    "hong kong",
    "taiwan", "taipei",
    "japan", "tokyo", "osaka", "yokohama", "kyoto",
    "south korea", "korea", "seoul", "busan",
    "singapore",
    "malaysia", "kuala lumpur",
    "indonesia", "jakarta",
    "thailand", "bangkok",
    "vietnam", "ho chi minh", "hanoi", "ho chi minh city",
    "philippines", "manila", "cebu",
    "pakistan", "karachi", "lahore", "islamabad",
    "bangladesh", "dhaka",
    "sri lanka", "colombo",

    "united arab emirates", "uae", "dubai", "abu dhabi",
    "saudi arabia", "riyadh", "jeddah",
    "israel", "tel aviv", "jerusalem", "haifa",
    "qatar", "doha",
    "bahrain", "manama",
    "kuwait", "kuwait city",
    "oman", "muscat",
    "jordan", "amman",
    "egypt", "cairo", "alexandria",
    "lebanon", "beirut",

    "south africa", "johannesburg", "cape town", "durban",
    "nigeria", "lagos", "abuja",
    "kenya", "nairobi",
    "ghana", "accra",
    "morocco", "casablanca", "rabat",
    "tunisia", "tunis",
    "ethiopia", "addis ababa",
    "rwanda", "kigali",
    "uganda", "kampala",
    "tanzania", "dar es salaam",

    "mexico", "mexican", "mexico city", "guadalajara", "monterrey",
    "brazil", "brazilian", "são paulo", "sao paulo", "rio de janeiro",
    "belo horizonte", "brasilia", "brasília",
    "argentina", "buenos aires", "córdoba", "cordoba",
    "chile", "santiago",
    "colombia", "bogotá", "bogota", "medellín", "medellin",
    "peru", "lima",
    "uruguay", "montevideo",
    "venezuela", "caracas",
    "costa rica", "san josé", "san jose, costa rica",
    "panama", "panama city",
    "dominican republic", "santo domingo",
    "ecuador", "quito", "guayaquil",
    "bolivia", "la paz",
    "paraguay", "asunción",
    "guatemala", "guatemala city",
    "el salvador", "san salvador",
    "honduras", "tegucigalpa",
    "nicaragua", "managua",
    "jamaica", "kingston",
    "trinidad and tobago",
    "barbados",
    "bahamas",
    "cuba", "havana",
    "haiti", "port-au-prince",

    "australia", "australian", "sydney", "melbourne", "brisbane",
    "perth", "adelaide", "canberra",
    "new zealand", "auckland", "wellington", "christchurch",
    "fiji",
]


def _word_regex(tokens: list[str]) -> re.Pattern | None:
    """Whole-word alternation regex, longest tokens first so multi-word
    tokens win over their substrings (e.g. "united states" over "states")."""
    seen: set[str] = set()
    escaped = []
    for t in tokens:
        t = t.strip().lower()
        if not t or t in seen:
            continue
        seen.add(t)
        escaped.append(re.escape(t))
    if not escaped:
        return None
    escaped.sort(key=len, reverse=True)
    pattern = r"(?:^|[\W_])(?:" + "|".join(escaped) + r")(?:[\W_]|$)"
    return re.compile(pattern, re.IGNORECASE)


_USA_RE = _word_regex(USA_TOKENS)
_NON_USA_RE = _word_regex(NON_USA_TOKENS)


def _split_parts(raw: str) -> list[str]:
    return [p.strip() for p in _PART_SPLITTER.split(raw) if p.strip()]


def _classify_part(part: str) -> str:
    padded = " " + part.lower() + " "
    if _NON_USA_RE is not None and _NON_USA_RE.search(padded):
        return NON_USA
    if _USA_RE is not None and _USA_RE.search(padded):
        return USA
    m = _STATE_SUFFIX.search(part)
    if m and m.group(1).upper() in US_STATE_CODES:
        return USA
    return UNKNOWN


def classify(raw: str) -> str:
    """Classify a free-text location string as 'usa', 'non_usa', or 'unknown'."""
    if not raw or not raw.strip():
        return UNKNOWN

    parts = _split_parts(raw)
    if not parts:
        return UNKNOWN

    saw_usa = False
    saw_non_usa = False
    for part in parts:
        result = _classify_part(part)
        if result == USA:
            saw_usa = True
        elif result == NON_USA:
            saw_non_usa = True

    if saw_usa:
        return USA
    if saw_non_usa:
        return NON_USA
    return UNKNOWN


def is_usa(raw: str) -> bool:
    return classify(raw) == USA
