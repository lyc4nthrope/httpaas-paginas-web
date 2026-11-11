#!/bin/bash
set -e
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

python3 -c "
import os, zipfile
d = '$DIR'

with zipfile.ZipFile(os.path.join(d, 'paisajes.zip'), 'w') as z:
    z.writestr('index.html', '<html><body><h1>Sitio de Paisajes</h1><img src=\"paisaje.jpg\"/></body></html>')
    z.writestr('paisaje.jpg', 'fake-image-data')

with zipfile.ZipFile(os.path.join(d, 'negocios.zip'), 'w') as z:
    z.writestr('negocios/index.html', '<html><body><h1>Sitio de Negocios</h1></body></html>')
    z.writestr('negocios/css/style.css', 'body { background: #eee; }')

with zipfile.ZipFile(os.path.join(d, 'sin-index.zip'), 'w') as z:
    z.writestr('archivo.txt', 'archivo de prueba')

with zipfile.ZipFile(os.path.join(d, 'zipslip.zip'), 'w') as z:
    z.writestr('../../tmp/x', 'malicioso')

with zipfile.ZipFile(os.path.join(d, 'symlink.zip'), 'w') as z:
    z.writestr('real.txt', 'real')
    zi = zipfile.ZipInfo('link.txt')
    zi.external_attr = 0o120777 << 16
    z.writestr(zi, 'real.txt')
"
echo "Archivos zip generados correctamente en $DIR"
