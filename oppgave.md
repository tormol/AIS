# AIS datastrømmer, lagring og APIer

## Bakgrunn

Store båter sender ut AIS informasjon om b.la. posisjon og størrelse via VHF. Denne informasjonen er ukryptert og kan fanges opp med en enkel mottaker. Kystverket har mottakere langs kysten og deler ut en samlet AIS datastrøm via internet. ECC har også tilgang til andre slike AIS datastrømmer.

## Oppgave

I denne oppgaven skal det lages en tjeneste som kan håndtere informasjon fra en eller flere slike AIS datastrømmer, lagre informasjonen på en effektiv måte og tilby forskjellige tjenester basert på oppsamlet informasjon.

Det må være mulig å ta i mot flere datastrømmer.

* Duplikate meldinger bør ignoreres (innenfor et smalt tidsvindu).
* Tjenesten bør kunne dele ut den flettede datastrømmen for lenkede tjenester.
* Håndtering av datastrømmer må være robust og håndtere reconnect osv.
* Datastrømmer bør kunne lese via UDP, TCP (brukes av Kystverket), HTTP (for lenkede tjenester)
* Tjenesten bør raskt kunne gi ut informasjon til klienter om siste registrerte posisjon for båtene. Enten alle eller innenfor en angitt bounding box.
* Tjenesten bør raskt kunne gi ut historisk informasjon til klienter om tracklog til en båt eller noen båter.
* Tjenesten bør inneholde et eksempel webkart som viser bruk av APIene som tjenesten tilbyr.
* Tjenesten bør implementeres med asynkron IO for å håndtere stort meldingsvolum og mange samtidige brukere av tjenesten.

ECC har flere løsninger for håndtering av AIS data pr i dag, men det er et stort forbedringspotensiale.

## Fagpersoner

Veileder: Tore Halset, [ECC Electronic Chart Centre](https://www.ecc.no/about-ecc)

Faglig Ansvarlig: Hein Meling
