# Troubleshooting

Common issues and solutions for DB Provision Operator.

## Diagnostic Commands

### Quick Health Check

```bash
# Operator status
kubectl get pods -n db-provision-operator-system

# Resource status
kubectl get databaseinstances,databases,databaseusers -A

# Recent events
kubectl get events -n db-provision-operator-system --sort-by='.lastTimestamp' | tail -20
```

### Detailed Debugging

```bash
# Operator logs
kubectl logs -n db-provision-operator-system deployment/db-provision-operator -f

# Specific resource details
kubectl describe databaseinstance postgres-primary

# Resource YAML
kubectl get databaseinstance postgres-primary -o yaml
```

## Common Issues

### DatabaseInstance Issues

#### Instance Stuck in Pending

**Symptoms:**
```
NAME              ENGINE    PHASE
postgres-primary  postgres  Pending
```

**Causes and Solutions:**

1. **Cannot connect to database:**
   ```bash
   # Check connectivity
   kubectl run psql-test --rm -it --image=postgres:15 -- \
     psql "postgresql://user:pass@host:5432/postgres?sslmode=disable" -c "SELECT 1"
   ```

2. **Invalid credentials:**
   ```bash
   # Verify secret exists and has correct keys
   kubectl get secret postgres-admin-credentials -o yaml
   ```

3. **DNS resolution:**
   ```bash
   # Test DNS from within cluster
   kubectl run dns-test --rm -it --image=busybox -- nslookup postgres.database.svc.cluster.local
   ```

#### Instance Shows Failed

**Check events:**
```bash
kubectl describe databaseinstance postgres-primary | grep -A10 "Events:"
```

**Common errors:**

| Error | Solution |
|-------|----------|
| `connection refused` | Check database is running, correct port |
| `authentication failed` | Verify credentials in Secret |
| `SSL required` | Add `sslMode: require` to spec |
| `unknown host` | Check hostname and DNS |

### Database Issues

#### Database Not Created

**Check dependencies:**
```bash
# Instance must be Ready
kubectl get databaseinstance postgres-primary -o jsonpath='{.status.phase}'
```

**Check logs:**
```bash
kubectl logs -n db-provision-operator-system deployment/db-provision-operator | grep "myapp-database"
```

#### Extension Installation Failed

**Common causes:**

1. Extension not available:
   ```sql
   -- Check available extensions in database
   SELECT * FROM pg_available_extensions;
   ```

2. Insufficient permissions:
   ```sql
   -- Admin user needs CREATE permission
   GRANT CREATE ON DATABASE myapp TO admin_user;
   ```

### User Issues

#### User Not Created

**Check instance status:**
```bash
kubectl get databaseinstance -o jsonpath='{.status.phase}'
# Must be "Ready"
```

**Verify username is valid:**
- PostgreSQL: alphanumeric and underscores
- MySQL: max 32 characters

#### Password Not Generated

**Check secret:**
```bash
kubectl get secret myapp-user-credentials
```

**If missing:**
1. Verify `passwordSecret.generate: true` in spec
2. Check `passwordSecret.secretName` is specified
3. Look for errors in operator logs

#### Cannot Connect with Generated Password

**Verify credentials:**
```bash
# Get password
kubectl get secret myapp-user-credentials -o jsonpath='{.data.password}' | base64 -d

# Test connection
kubectl run psql-test --rm -it --image=postgres:15 -- \
  psql "postgresql://myapp_user:PASSWORD@postgres:5432/myapp"
```

### Grant Issues

#### Grants Not Applied

**Prerequisites:**
1. User/Role must exist and be Ready
2. Database must exist
3. Tables/schemas must exist

**Check user status:**
```bash
kubectl get databaseuser myapp-user -o jsonpath='{.status.phase}'
```

**Verify grants in database:**
```sql
-- PostgreSQL
\dp tablename
-- or
SELECT * FROM information_schema.table_privileges WHERE grantee = 'myapp_user';
```

### Backup Issues

#### Backup Stuck in Running

**Check backup job:**
```bash
kubectl get jobs -l backup-name=myapp-backup
kubectl logs job/myapp-backup-job
```

**Common causes:**
- Large database taking long time
- Network issues to storage
- Insufficient resources

#### Backup Failed

**Check job logs:**
```bash
kubectl logs job/myapp-backup-job
```

**Storage issues:**
```bash
# Verify storage credentials
kubectl get secret s3-credentials -o yaml

# Test S3 connectivity
kubectl run aws-cli --rm -it --image=amazon/aws-cli -- s3 ls s3://my-bucket/
```

#### Schedule Not Triggering

**Verify schedule:**
```bash
kubectl get databasebackupschedule myapp-backup -o yaml | grep schedule
```

**Check timezone:**
```bash
# Schedule uses UTC by default
kubectl get databasebackupschedule myapp-backup -o jsonpath='{.spec.timezone}'
```

### Restore Issues

#### Restore Failed

**Check restore job:**
```bash
kubectl logs job/myapp-restore-job
```

**Common errors:**

| Error | Solution |
|-------|----------|
| `database exists` | Set `dropExisting: true` or delete database |
| `backup not found` | Verify backup exists and path is correct |
| `permission denied` | Check storage credentials |

## Operator Issues

### Operator Not Starting

**Check pod status:**
```bash
kubectl describe pod -n db-provision-operator-system -l app=db-provision-operator
```

**Common issues:**

1. **Image pull error:**
   ```bash
   kubectl get events -n db-provision-operator-system | grep "Failed"
   ```

2. **RBAC issues:**
   ```bash
   kubectl auth can-i --as=system:serviceaccount:db-provision-operator-system:db-provision-operator \
     create secrets -n default
   ```

### High Memory Usage

**Check resource usage:**
```bash
kubectl top pod -n db-provision-operator-system
```

**Solutions:**
1. Increase memory limits
2. Reduce concurrent reconciles
3. Check for resource leaks in logs

### Reconciliation Loops

**Symptoms:** Constant reconciliation, high CPU

**Debug:**
```bash
# Watch reconciliation
kubectl logs -n db-provision-operator-system deployment/db-provision-operator -f | grep "Reconciling"
```

**Causes:**
- Status updates triggering reconciles
- External changes to managed resources
- Conflicting controllers

## Recovery Procedures

### Force Delete Stuck Resource

```bash
# Remove finalizers (use with caution!)
kubectl patch database myapp-database -p '{"metadata":{"finalizers":null}}' --type=merge
kubectl delete database myapp-database
```

### Reset Resource State

```bash
# Delete and recreate
kubectl delete database myapp-database
kubectl apply -f database.yaml
```

### Operator Recovery

```bash
# Restart operator
kubectl rollout restart deployment/db-provision-operator -n db-provision-operator-system

# Watch rollout
kubectl rollout status deployment/db-provision-operator -n db-provision-operator-system
```

## Debug Mode

### Enable Debug Logging

```yaml
# Helm values
logging:
  level: debug
```

Or patch deployment:
```bash
kubectl set env deployment/db-provision-operator LOG_LEVEL=debug -n db-provision-operator-system
```

### Trace Specific Resource

```bash
kubectl logs -n db-provision-operator-system deployment/db-provision-operator | \
  grep "myapp-database"
```

## Getting Help

### Collect Debug Information

```bash
#!/bin/bash
# debug-bundle.sh

echo "=== Operator Pods ===" > debug.txt
kubectl get pods -n db-provision-operator-system -o wide >> debug.txt

echo "=== Operator Logs ===" >> debug.txt
kubectl logs -n db-provision-operator-system deployment/db-provision-operator --tail=500 >> debug.txt

echo "=== All Resources ===" >> debug.txt
kubectl get databaseinstances,databases,databaseusers,databaseroles,databasegrants,databasebackups -A >> debug.txt

echo "=== Events ===" >> debug.txt
kubectl get events -n db-provision-operator-system >> debug.txt

echo "Debug bundle saved to debug.txt"
```

### Report Issues

Open an issue at: https://github.com/panteparak/db-provision-operator/issues

Include:
1. Operator version
2. Kubernetes version
3. Database engine and version
4. Resource YAML (redact secrets)
5. Operator logs
6. Steps to reproduce
