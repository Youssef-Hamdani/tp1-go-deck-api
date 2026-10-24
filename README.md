# TP1 :  API de cartes en Go

## Démarrage

Prérequis : Go ≥ 1.24 , GCC et CGO activé ainsi que le framework `github.com/mattn/go-sqlite3 v1.14.52`.

```bash
git clone https://github.com/Youssef-Hamdani/tp1-go-deck-api.git
cd tp1-go-deck-api
go mod download
export CGO_ENABLED=1
go run main.go
```

### créer un deck

```bash
curl 'http://localhost:8080/api/deck/new/'
curl 'http://localhost:8080/api/deck/new/shuffle/?deck_count=6'
curl 'http://localhost:8080/api/deck/new/?jokers_enabled=true'
curl 'http://localhost:8080/api/deck/new/shuffle/?cards=AS,2S,KS,AH'
curl 'http://localhost:8080/api/deck/new/draw/?count=2'
```

Remplacer `ID` par le `deck_id` reçu :

```bash
curl 'http://localhost:8080/api/deck/ID/draw/?count=3'
curl 'http://localhost:8080/api/deck/ID/pile/discard/add/?cards=AS,2S'
curl 'http://localhost:8080/api/deck/ID/pile/discard/list/'
curl 'http://localhost:8080/api/deck/ID/pile/discard/draw/'
curl 'http://localhost:8080/api/deck/ID/pile/discard/draw/top/?count=1'
curl 'http://localhost:8080/api/deck/ID/pile/discard/draw/bottom/'
curl 'http://localhost:8080/api/deck/ID/pile/discard/draw/random/'
curl 'http://localhost:8080/api/deck/ID/pile/discard/draw/?cards=AS'
curl 'http://localhost:8080/api/deck/ID/pile/discard/shuffle/'
curl 'http://localhost:8080/api/deck/ID/pile/discard/return/?cards=2S'
curl 'http://localhost:8080/api/deck/ID/pile/discard/return/'
curl 'http://localhost:8080/api/deck/ID/return/?cards=AS'
curl 'http://localhost:8080/api/deck/ID/return/'
curl 'http://localhost:8080/api/deck/ID/shuffle/?remaining=true'
curl 'http://localhost:8080/api/deck/ID/shuffle/'
```

Ces commandes illustrent les routes; une carte doit réellement être en main
pour être ajoutée à une pile, ou présente dans la pile pour en être retirée.
Le scénario `./scripts/demo.sh` enchaîne des opérations compatibles entre elles.
Les opérations de pile sont interdites si `deck_count > 1`.

## Configuration et arrêt

```bash
go run main.go -addr :8080 -db deck.db -readers 3 -timeout 2s
# Derrière un proxy, définir explicitement l'origine publique des images :
go run main.go -base-url https://cartes.example.org
```

| Option | Défaut | Rôle |
|---|---|---|
| `-addr` | `:8080` | Adresse d'écoute |
| `-db` | `deck.db` | Fichier SQLite persistant |
| `-readers` | `3` | 1 à 3 workers de lecture |
| `-timeout` | `2s` | Délai de chaque requête |
| `-base-url` | origine de la requête | Préfixe des liens SVG |

`Ctrl+C`/SIGTERM arrête l'admission HTTP, attend les requêtes, puis l'historique,
ferme les channels et attend les workers avant de fermer SQLite. Utiliser **un
seul processus serveur par base** : le cache et l'unique écrivain sont locaux au
processus. Les `.db`, binaires et caches ne sont pas suivis par Git.

## Images et conventions

Ouvrir `/static/img/AS.svg`, `/static/img/10H.svg`, `/static/img/X1.svg`,
`/static/img/X2.svg` et `/static/img/back.svg`. Les 52 cartes et les deux jokers proviennent du dépôt officiel

Les codes JSON utilisent `10H`; l'entrée `0H` est acceptée et normalisée.
L'ordre neuf est A→K, PIQUE, CARREAU, TREFLE, COEUR. Les retours sont ajoutés
au bas du deck dans un ordre stable. Un mélange complet récupère toutes les
cartes, y compris les piles; `remaining=true` préserve les mains et piles.
Les limites choisies sont 1–20 paquets, 1–1080 cartes par pige, noms de pile
non vides de 100 octets au plus. Voir les autres décisions dans `ARCHITECTURE.md`.

## Vérification et démonstration

```bash
./scripts/verify.sh
# Dans un terminal, serveur instrumenté :
go run -race main.go
# Dans un autre :
./scripts/demo.sh
./scripts/load.sh                 # 104 requêtes simultanées
REQUESTS=52 ./scripts/load.sh     # 52 cartes distinctes exactement
```

Le client de démonstration est lui aussi en Go standard (`go run ./cmd/demo`).
`BASE_URL=http://localhost:8081 ./scripts/demo.sh` cible une autre adresse.
Les scripts doivent être exécutés avec le Go et CGO préparés sur votre machine.

Les résultats réellement mesurés, la méthode et les sorties brutes sont dans
`docs/VERIFICATION_REPORT.md` et `docs/verification/`. Les tests comprennent
52 piges concurrentes uniques, 104 piges réussies sur deux paquets, une charge
HTTP de 104 requêtes sur un même paquet, l'isolation, les délais, les transactions,
la fermeture, la persistance, l'historique et les 55 SVG.

## Mesures

Go 1.27.3, Linux x86 Intel(R) Core(TM) i5-10400F CPU @ 2.90GHz avec 3 lecteurs, SQLite WAL sur disque,
historique activé. Cinq vagues de 104 requêtes HTTP donnent **2132,94 à 4692,77
req/s**, médiane **3242,67 req/s**. Chaque vague contient 52 succès et 52 erreurs
« Deck is empty » attendues; ce débit ne représente pas 104 piges réussies.

`BenchmarkDrawOne` : **337270 ns/op**, 36333 B/op, 523 allocations/op (3501
itérations). La création du deck est hors mesure, chaque série épuise 52 cartes.
`BenchmarkDrawReturnCycle` : **695475 ns/cycle**, 74640 B/op, 1235 allocations/cycle
(2052 cycles; une pige puis un retour). Les deux passent par les handlers et
incluent le décodage des réponses par le banc de test, sans TCP.

Sur le serveur lancé avec `go run -race main.go`, le script complet mesure
**422,90 req/s** pour 104 requêtes; la vague séparée de 52 piges réussies mesure
**143,14 req/s**. Les coûts d'instrumentation et la charge cloud expliquent que
ces chiffres ne soient pas directement comparables aux tests sans `-race`.

