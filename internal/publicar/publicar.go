package publicar

import (
"crypto/rand"
"encoding/hex"
"fmt"
"net/http"
"strings"
"time"
)

type Executor interface {
CopyTo(srcLocal, dstRemote string) error
Run(cmd string) (exitCode int, stdout string, stderr string, err error)
}

type Publicador struct {
Exec Executor
}

func NewPublicador(exec Executor) *Publicador {
return &Publicador{Exec: exec}
}

func (p *Publicador) Publicar(host, ip, zipPath string) error {
bytes := make([]byte, 8)
if _, err := rand.Read(bytes); err != nil {
return fmt.Errorf("error al generar id aleatorio: %w", err)
}
id := hex.EncodeToString(bytes)
destRemote := fmt.Sprintf("/home/httpaas/subidas/%s.zip", id)

if err := p.Exec.CopyTo(zipPath, destRemote); err != nil {
return fmt.Errorf("error al copiar zip a la vm: %w", err)
}

cmd := fmt.Sprintf("sudo /usr/local/sbin/httpaas-publicar %s", destRemote)
exitCode, stdout, stderr, err := p.Exec.Run(cmd)
if err != nil && exitCode == 0 {
return fmt.Errorf("error de ejecucion remota: %w", err)
}

switch exitCode {
case 0:
// Exito
case 2:
return fmt.Errorf("error de validacion en VM (codigo 2): %s %s", stdout, stderr)
case 3:
return fmt.Errorf("error interno de servidor en VM (codigo 3): %s %s", stdout, stderr)
default:
return fmt.Errorf("error desconocido en VM (codigo %d): %s %s", exitCode, stdout, stderr)
}

reqURL := fmt.Sprintf("http://%s/", ip)
req, err := http.NewRequest("GET", reqURL, nil)
if err != nil {
return fmt.Errorf("error al crear solicitud de verificacion http: %w", err)
}

domainHost := host
if !strings.Contains(host, ".") {
domainHost = fmt.Sprintf("%s.cloud.local", host)
}
req.Host = domainHost

client := &http.Client{Timeout: 5 * time.Second}
resp, err := client.Do(req)
if err != nil {
return fmt.Errorf("error al verificar sitio web http: %w", err)
}
defer resp.Body.Close()

if resp.StatusCode != http.StatusOK {
return fmt.Errorf("verificacion http fallo con codigo estado: %d", resp.StatusCode)
}

return nil
}