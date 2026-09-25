# Generates webhook TLS material and (if a cluster is available) applies Secret + caBundle.
#
# Usage (from repo root):
#   .\scripts\gen-webhook-certs.ps1

$ErrorActionPreference = "Stop"

$Namespace = "backup-system"
$SecretName = "webhook-server-cert"
$OutDir = Join-Path $PSScriptRoot "..\config\webhook\certs"
$WebhookConfig = "backup-operator-validating"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")

Push-Location $RepoRoot
try {
  go run ./scripts/gencerts -out $OutDir
} finally {
  Pop-Location
}

$caPath = Join-Path $OutDir "ca.crt"
$crtPath = Join-Path $OutDir "tls.crt"
$keyPath = Join-Path $OutDir "tls.key"

if (-not (Get-Command kubectl -ErrorAction SilentlyContinue)) {
  Write-Host "Certs ready under $OutDir (kubectl missing; skip cluster apply)"
  exit 0
}

kubectl cluster-info 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) {
  Write-Host "Certs ready under $OutDir (no Kubernetes cluster; skip apply)"
  exit 0
}

kubectl get ns $Namespace 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) {
  kubectl create namespace $Namespace
}

kubectl -n $Namespace delete secret $SecretName --ignore-not-found | Out-Null
kubectl -n $Namespace create secret tls $SecretName --cert=$crtPath --key=$keyPath

# caBundle must be base64 of the CA certificate DER (or PEM). PEM bytes base64-encoded is accepted.
$caBundle = [Convert]::ToBase64String([IO.File]::ReadAllBytes($caPath))

kubectl apply -f (Join-Path $RepoRoot "config\webhook\service.yaml")
kubectl apply -f (Join-Path $RepoRoot "config\webhook\manifests.yaml")

$patch = @"
[
  {"op":"replace","path":"/webhooks/0/clientConfig/caBundle","value":"$caBundle"},
  {"op":"replace","path":"/webhooks/1/clientConfig/caBundle","value":"$caBundle"}
]
"@
$patchFile = Join-Path $OutDir "cabundle-patch.json"
Set-Content -Path $patchFile -Value $patch -Encoding ascii
kubectl patch validatingwebhookconfiguration $WebhookConfig --type=json --patch-file $patchFile

Write-Host "Done. Secret $SecretName and caBundle updated."
