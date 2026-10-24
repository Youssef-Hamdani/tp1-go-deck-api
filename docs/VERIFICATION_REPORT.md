# Rapport de vérification : 18 octobre 2026

## Résultat

Implémentation complète des routes de l'énoncé, compilée et testée sur
l'environnement linux, sans réinstallation. Les tests normaux et
instrumentés passent; `go vet` réussit et `gofmt -l .` est vide. Le serveur a
réellement exécuté le scénario complet et deux charges concurrentes sous
`go run -race main.go`, puis s'est arrêté proprement. Aucune `DATA RACE`
n'apparaît dans les journaux.

## Commandes exécutées et preuves


| Commande | Résultat | Preuve |
|---|---|---|
| `go mod tidy` | Succès; seul module externe mattn/go-sqlite3 v1.14.52 | `go.mod`, `go.sum`, script complet |
| `go test ./... -count=1` | Tous les packages de tests passent | `verification/tests.txt` |
| `go test -race ./... -count=1` | Tous passent, aucune race | `verification/race.txt` |
| `go vet ./...` | Code de sortie 0, sortie vide | `verification/vet.txt` |
| `gofmt -l .` | Sortie vide | `verification/gofmt.txt` |
| `bash -n scripts/demo.sh scripts/load.sh scripts/verify.sh` | Succès | Exécution cloud |
| `./scripts/verify.sh` | Succès de toute la séquence, dont charge répétée et benchmarks | `verification/verify-script.txt` |
| `go build -o work/deck-api .` | Succès; binaire ignoré par Git | Exécution cloud |
| `go run -race main.go -addr 127.0.0.1:18080 -db work/live-final.db` | Serveur réellement lancé, SIGTERM au processus serveur, sortie 0 | `verification/live-check.txt`, `live-race.txt` |
| `BASE_URL=http://127.0.0.1:18080 ./scripts/demo.sh` | Succès, API complète + charge 104 | `verification/live-demo.txt` |
| `BASE_URL=http://127.0.0.1:18080 REQUESTS=52 ./scripts/load.sh` | 52 cartes uniques, retour à 52 | `verification/live-load52.txt` |

Environnement observé : Go **1.27.3**, CGO_ENABLED=**1**, Linux/x86,
Intel(R) Core(TM) i5-10400F CPU @ 2.90GHz. Le suffixe `-3` des benchmarks indique GOMAXPROCS=3. Le fichier `verification/environment.txt` conserve la version.
GCC/CGO/SQLite n'ont pas été reconfigurés; la compilation et l'exécution du
pilote confirment leur utilisation effective.

## Matrice de conformité et tests

| Exigence | Tests / preuves concrètes |
|---|---|
| Deck neuf, ordre A–K et suites S/D/C/H | `TestDeckContractAndOrder`, `TestStandardAndFrench` : comparaison des 52 codes dans l'ordre |
| Mélangé, multideck, jokers, partiel et combinaisons | `TestCreationVariants` : 52, 312, 54, 108 et sélection partielle répétée |
| Création+pige atomique avec `new` | `TestCreationVariants`, `TestAtomicNewDrawFailureAndFastCache` : échec de pige sans deck résiduel |
| Codes français, `10`/`0`, jokers | `TestCodes`, `TestStandardAndFrench`, `TestCreationVariants`; origine X1/X2 confirmée dans le code amont |
| JSON de deck/draw et champs optionnels | `TestDeckContractAndOrder`, `TestCreationVariants`, `TestPileLifecycle` : absence de shuffled/cards/images non requis, bons décomptes et liens |
| Pige par ordre persistant | `TestConcurrentDraw52`, `TestConcurrentDrawBatches` : 52 codes exacts, décomptes consécutifs, 13 lots de 4 cartes contiguës |
| Mélange complet et `remaining=true` | `TestShuffleAndReturnSemantics` : main et pile conservées en mode remaining, tout récupéré en complet |
| Ajout de pile seulement depuis la main | `TestPileLifecycle`, `TestErrors`, `TestAtomicFailedTransfer` |
| Création automatique et liste de toutes les piles | `TestPileLifecycle` : détails de la pile demandée, seuls décomptes des autres; pile vide conservée |
| Pige LIFO/FIFO, top explicite et random | `TestPileLifecycle`, `TestPileCountVariantsAndIsolation` : sorties exactes top/bottom et sous-ensemble unique random de trois cartes |
| Pige par codes et mélange de pile | `TestPileLifecycle`, `TestPileCountVariantsAndIsolation` : cartes choisies, conservation et isolation des autres piles/deck |
| Retour de toutes / certaines cartes en main | `TestPileLifecycle`, `TestShuffleAndReturnSemantics` : ordre stable au bas et sélection multideck déterministe |
| Retour de toutes / certaines cartes de pile | `TestPileLifecycle` : décompte pile/deck, main séparée |
| Refus de piles en multideck | `TestErrors`, `TestAtomicNewDrawFailureAndFastCache` : refus depuis cache même pool fermé |
| Tous les messages d'erreur prescrits | `TestErrors`, `TestDeckContractAndOrder`, `TestPileLifecycle`, `TestHTTPDatabaseError`, `TestMiddlewareAndHistory`, `TestTimeoutQueuedJob` |
| Atomicité d'une sélection partiellement invalide | `TestAtomicFailedTransfer` : aucune carte déplacée ni pile créée |
| 52 + 2 jokers + dos, XML et MIME SVG | `TestEverySVG` : 55 réponses 200, `image/svg+xml`, namespace et XML valides; 404 si fichier absent |
| WAL et 1–3 lecteurs configurables | `TestWALAndConfiguration` : mode WAL lu en SQL et rejet des valeurs 0/4 |
| Lectures réellement parallèles, écrivain unique | `TestReaderParallelismAndSingleWriter` : trois lecteurs dans le job en même temps, maximum d'un écrivain actif |
| Channels de réponse uniques | `TestUniqueReplies` : chaque demandeur récupère sa propre valeur parmi 100 jobs concurrents |
| Cache RWMutex préchargé et persistance | `TestPersistenceAndCacheReload` : fermeture/réouverture et reprise des cartes et piles; accès concurrents sous race detector |
| Délai HTTP 503 et abandon avant exécution | `TestTimeoutQueuedJob` : écrivain bloqué, délai 10ms, prochaine carte toujours AS; `TestExpiredQueuedJobSkipped` vérifie que le callback expiré ne tourne pas |
| Charge 100+ sur le même deck | `TestConcurrentLoad104` via vrai serveur `httptest` : 104 requêtes, 52 succès uniques + 52 erreurs de deck vide |
| Conservation de 104 cartes physiques | `TestConcurrentMultiDeck104` : 104 succès concurrents, chaque code exactement deux fois, retour à 104 |
| Isolation de plusieurs decks via handlers | `TestIsolation` : huit decks, dix piges ordonnées puis retour, aucune interférence |
| Fermeture sans worker bloqué, soumission concurrente | `TestConcurrentClose` : 300 soumissions/Close concurrent, délai de sécurité, rejet après fermeture; tous les tests ferment le pool |
| Panique du handler et du worker | `TestMiddlewareAndHistory`, `TestWorkerPanicDoesNotKillPool` : JSON 500 attendu et pool encore utilisable |
| Middleware ID typé, compteur unique, délai 2s | `TestMiddlewareAndHistory` : 100 ID parallèles distincts et deadline visible |
| Historique asynchrone, WithoutCancel, ID conservé | `TestMiddlewareAndHistory`, `TestHistoryDoesNotDelayResponse` : contexte parent annulé, écrivain bloqué, planification non bloquante et écriture finale |
| Arrêt HTTP → historique → pool | Exécution réelle sous `-race`, sortie 0, journal SQLite après fermeture : 125 lignes et 125 ID distincts |
| Intégrité SQLite | `PRAGMA integrity_check` = `ok` dans la charge automatisée et après la démonstration réelle |
| Documentation et scripts | README, ARCHITECTURE, présent rapport, DEMO_GUIDE, trois scripts exécutables; scénario réel lancé |


## Mesures de charge

Méthode : base SQLite temporaire **sur disque**, WAL, 3 lecteurs, historique
activé. Un deck standard non mélangé est créé avant la mesure. 104 goroutines
attendent une barrière, puis envoient chacune une requête HTTP de pige d'une
carte au même deck sur `httptest.NewServer`. La mesure `time.Now`/`time.Since`
inclut le lancement des goroutines, TCP, traitements et décodages. Le retour
final des cartes et l'attente complète de l'historique ne sont pas inclus dans
la durée. Le débit compte toutes les réponses

Cinq vagues du script livré (`verification/verify-script.txt`) :

| Vague | Temps pour 104 requêtes | Requêtes/s |
|---|---:|---:|
| 1 | 48,759014 ms | 2132,94 |
| 2 | 22,161730 ms | 4692,77 |
| 3 | 38,538782 ms | 2698,58 |
| 4 | 32,072305 ms | 3242,67 |
| 5 | 30,064598 ms | 3459,22 |

Médiane : **3242,67 req/s**; plage : **2132,94–4692,77 req/s**. Chaque vague
vérifie exactement 52 cartes distinctes et 52 erreurs `Deck is empty`, puis
le retour à 52 cartes. Le test multideck vérifie séparément 104 succès.

Serveur réellement lancé par `go run -race main.go`, scripts livrés :

- 104 requêtes : **245,922863 ms**, **422,90 req/s**, 52 succès + 52 erreurs attendues.
- 52 requêtes : **363,286139 ms**, **143,14 req/s**, 52 succès distincts, aucune erreur.
- Après arrêt : `integrity_check=ok`; 125 entrées d'historique avec 125 ID distincts.
  690 cartes physiques conservées dans la base du scénario : 688 en deck,
  2 en main du deck créé par `new/draw`. Aucune pile n'en retient à la fin.

## Benchmarks

Commande exécutée dans `./scripts/verify.sh` :
`go test -bench=. -benchmem ./...`.

| Benchmark | Itérations | Temps | Mémoire | Allocations |
|---|---:|---:|---:|---:|
| `BenchmarkDrawOne-3` | 3501 | 337270 ns/op | 36333 B/op | 523 allocs/op |
| `BenchmarkDrawReturnCycle-3` | 2052 | 695475 ns/op | 74640 B/op | 1235 allocs/op |

Les deux benchmarks passent par les vrais handlers, le pool et SQLite WAL.
Le helper du banc décode aussi le JSON; il n'y a pas de TCP dans ces mesures.
Pour `DrawOne`, création et attente de l'historique de création sont hors timer
chaque 52 cartes; chaque itération mesurée est une pige réussie. Le second
benchmark mesure un **cycle de deux opérations**, pige puis retour. L'historique

## Ressources

Les 54 SVG de cartes/jokers sont téléchargés du dépôt officiel, Les dix sont renommés `0H.svg → 10H.svg` sans changer les octets.