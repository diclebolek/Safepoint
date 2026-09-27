# =============================================================================
# Bu dosya ne ise yarar?
#   Production ipucu: DB sifrelerini BackupSchedule icine gomme.
#   Secret'i ayri olustur (CI/Vault/Sealed Secrets); schedule sadece secretRef kullansin.
#
# Ornek:
#   kubectl -n myapp create secret generic payments-db \
#     --from-literal=host=db.myapp.svc \
#     --from-literal=port=5432 \
#     --from-literal=username=app \
#     --from-literal=password='***' \
#     --from-literal=database=payments
#
# Sealed Secrets:
#   kubeseal < secret.yaml > sealed-secret.yaml && kubectl apply -f sealed-secret.yaml
# =============================================================================
