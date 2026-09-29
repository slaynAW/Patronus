# Logo

Logo « Topologie » : un commutateur relié à trois PC, dont un qui s'allume (en bleu). Couleurs de l'interface :
fond `#12161B`, bleu `#4A94FF`, gris `#6B7380` / `#3A4250`, commutateur `#E7E9EC`, voyant `#2FD27A`.

| Fichier | Usage |
|---|---|
| `logo.svg` | Logo de référence (48 px et plus), README |
| `logo-small.svg` | Variante simplifiée pour 32 px et moins (traits épais, sans halo ni voyant) |

Déclinaisons, à garder identiques au SVG :

- **Android** : icône adaptative `app/src/main/res/drawable/ic_launcher_foreground.xml` (fond : couleur
  `ic_launcher_background`), icône à thème `ic_launcher_monochrome.xml`, logo dans l'application `ic_logo.xml`.
- **Windows** : icône de l'exécutable `desktop/winres/icon*.png` (256, 48, 32, 24 et 16 px), générée par
  `node branding/render.mjs` (Playwright et Chromium nécessaires) ; logo de l'interface dans `desktop/ui/app.js` (`LOGO`).
