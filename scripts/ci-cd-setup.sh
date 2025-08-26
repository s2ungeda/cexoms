#!/bin/bash

# CI/CD setup script for mExOms

set -e

# Configuration
CI_SYSTEM=${1:-github}  # github, gitlab, jenkins
ENVIRONMENT=${2:-all}    # dev, staging, production, all

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Logging functions
log() { echo -e "${GREEN}[SETUP]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
info() { echo -e "${BLUE}[INFO]${NC} $1"; }

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    
    local required_tools=("kubectl" "helm" "docker")
    
    for tool in "${required_tools[@]}"; do
        if ! command -v $tool &> /dev/null; then
            error "$tool is not installed"
        fi
    done
    
    log "All prerequisites met"
}

# Setup GitHub Actions
setup_github() {
    log "Setting up GitHub Actions..."
    
    # Create secrets file template
    cat > .github/secrets-template.yml <<EOF
# GitHub Secrets Configuration
# Add these secrets to your repository settings

# Docker Registry
DOCKER_USERNAME: your-docker-username
DOCKER_PASSWORD: your-docker-password

# AWS Credentials
AWS_ACCESS_KEY_ID: your-aws-access-key
AWS_SECRET_ACCESS_KEY: your-aws-secret-key
PROD_AWS_ACCESS_KEY_ID: your-prod-aws-access-key
PROD_AWS_SECRET_ACCESS_KEY: your-prod-aws-secret-key

# Kubernetes Config
KUBECONFIG_DEV: base64-encoded-kubeconfig-dev
KUBECONFIG_STAGING: base64-encoded-kubeconfig-staging
KUBECONFIG_PROD: base64-encoded-kubeconfig-prod

# Slack Notifications
SLACK_WEBHOOK: your-slack-webhook-url
PROD_SLACK_WEBHOOK: your-prod-slack-webhook-url

# Code Analysis
CODECOV_TOKEN: your-codecov-token
SONAR_TOKEN: your-sonarcloud-token
EOF
    
    # Create environments
    info "Create the following environments in GitHub:"
    echo "  - development"
    echo "  - staging"
    echo "  - production (with required reviewers)"
    
    # Create workflow dispatch script
    cat > scripts/trigger-deployment.sh <<'EOF'
#!/bin/bash
# Trigger GitHub Actions deployment

ENVIRONMENT=${1:-staging}
BRANCH=${2:-main}

curl -X POST \
  -H "Accept: application/vnd.github.v3+json" \
  -H "Authorization: token $GITHUB_TOKEN" \
  https://api.github.com/repos/mexoms/mexoms/actions/workflows/cd.yml/dispatches \
  -d "{\"ref\":\"$BRANCH\",\"inputs\":{\"environment\":\"$ENVIRONMENT\"}}"
EOF
    
    chmod +x scripts/trigger-deployment.sh
}

# Setup GitLab CI
setup_gitlab() {
    log "Setting up GitLab CI..."
    
    # Create variables file template
    cat > .gitlab/variables-template.yml <<EOF
# GitLab CI/CD Variables
# Add these to your project settings

# Container Registry
CI_REGISTRY_USER: gitlab-ci-token
CI_REGISTRY_PASSWORD: \$CI_JOB_TOKEN

# Kubernetes
KUBE_URL: https://your-k8s-api-server
KUBE_TOKEN: your-service-account-token
KUBE_NAMESPACE: mexoms

# Environments
DEV_KUBE_URL: https://dev-k8s-api-server
STAGING_KUBE_URL: https://staging-k8s-api-server
PROD_KUBE_URL: https://prod-k8s-api-server
EOF
    
    # Create GitLab environments
    cat > .gitlab/environments.yml <<EOF
# GitLab Environments Configuration

development:
  name: development
  url: https://dev.mexoms.com
  kubernetes:
    namespace: mexoms-dev

staging:
  name: staging
  url: https://staging.mexoms.com
  kubernetes:
    namespace: mexoms-staging
    
production:
  name: production
  url: https://mexoms.com
  kubernetes:
    namespace: mexoms
  deployment_tier: production
  protected: true
EOF
}

# Setup Jenkins
setup_jenkins() {
    log "Setting up Jenkins..."
    
    # Create Jenkins configuration
    cat > jenkins/config.xml <<EOF
<?xml version='1.1' encoding='UTF-8'?>
<flow-definition plugin="workflow-job@2.40">
  <description>mExOms CI/CD Pipeline</description>
  <keepDependencies>false</keepDependencies>
  <properties>
    <org.jenkinsci.plugins.workflow.job.properties.PipelineTriggersJobProperty>
      <triggers>
        <hudson.triggers.SCMTrigger>
          <spec>H/5 * * * *</spec>
        </hudson.triggers.SCMTrigger>
      </triggers>
    </org.jenkinsci.plugins.workflow.job.properties.PipelineTriggersJobProperty>
    <org.jenkinsci.plugins.workflow.job.properties.DisableConcurrentBuildsJobProperty/>
  </properties>
  <definition class="org.jenkinsci.plugins.workflow.cps.CpsScmFlowDefinition" plugin="workflow-cps@2.90">
    <scm class="hudson.plugins.git.GitSCM" plugin="git@4.7.1">
      <configVersion>2</configVersion>
      <userRemoteConfigs>
        <hudson.plugins.git.UserRemoteConfig>
          <url>https://github.com/mexoms/mexoms.git</url>
        </hudson.plugins.git.UserRemoteConfig>
      </userRemoteConfigs>
      <branches>
        <hudson.plugins.git.BranchSpec>
          <name>*/main</name>
        </hudson.plugins.git.BranchSpec>
      </branches>
    </scm>
    <scriptPath>Jenkinsfile</scriptPath>
  </definition>
</flow-definition>
EOF
    
    # Create credentials template
    cat > jenkins/credentials-template.xml <<EOF
# Jenkins Credentials to Configure

1. Docker Registry (docker-registry)
   - Type: Username with password
   - Username: your-docker-username
   - Password: your-docker-password

2. Kubernetes Config (kubeconfig)
   - Type: Secret file
   - File: kubeconfig file

3. Slack Webhook (slack-webhook)
   - Type: Secret text
   - Secret: your-slack-webhook-url

4. SonarQube Token (sonar-token)
   - Type: Secret text
   - Secret: your-sonarqube-token
EOF
}

# Setup Kubernetes resources
setup_k8s_resources() {
    local env=$1
    log "Setting up Kubernetes resources for $env..."
    
    # Create namespace
    kubectl create namespace mexoms-$env --dry-run=client -o yaml | kubectl apply -f -
    
    # Create service account for CI/CD
    kubectl create serviceaccount ci-cd -n mexoms-$env --dry-run=client -o yaml | kubectl apply -f -
    
    # Create RBAC
    cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: ci-cd-role
  namespace: mexoms-$env
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ci-cd-rolebinding
  namespace: mexoms-$env
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: ci-cd-role
subjects:
- kind: ServiceAccount
  name: ci-cd
  namespace: mexoms-$env
EOF
    
    # Get service account token
    SA_TOKEN=$(kubectl get secret -n mexoms-$env \
        $(kubectl get sa ci-cd -n mexoms-$env -o jsonpath='{.secrets[0].name}') \
        -o jsonpath='{.data.token}' | base64 -d)
    
    info "Service account token for $env:"
    echo "$SA_TOKEN"
}

# Setup monitoring
setup_monitoring() {
    log "Setting up CI/CD monitoring..."
    
    # Create Grafana dashboard
    cat > monitoring/ci-cd-dashboard.json <<EOF
{
  "dashboard": {
    "title": "CI/CD Pipeline Metrics",
    "panels": [
      {
        "title": "Build Success Rate",
        "targets": [
          {
            "expr": "sum(rate(ci_pipeline_success_total[5m])) / sum(rate(ci_pipeline_total[5m]))"
          }
        ]
      },
      {
        "title": "Average Build Time",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, ci_pipeline_duration_seconds_bucket)"
          }
        ]
      },
      {
        "title": "Deployment Frequency",
        "targets": [
          {
            "expr": "sum(rate(deployments_total[1d]))"
          }
        ]
      }
    ]
  }
}
EOF
    
    # Create alert rules
    cat > monitoring/ci-cd-alerts.yml <<EOF
groups:
- name: ci_cd_alerts
  rules:
  - alert: HighBuildFailureRate
    expr: |
      (
        sum(rate(ci_pipeline_failed_total[30m]))
        /
        sum(rate(ci_pipeline_total[30m]))
      ) > 0.25
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: High build failure rate detected
      description: "Build failure rate is {{ \$value | humanizePercentage }}"
      
  - alert: LongBuildTime
    expr: |
      histogram_quantile(0.95, ci_pipeline_duration_seconds_bucket) > 1800
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: Build taking too long
      description: "95th percentile build time is {{ \$value | humanizeDuration }}"
EOF
}

# Main setup flow
main() {
    check_prerequisites
    
    case "$CI_SYSTEM" in
        github)
            setup_github
            ;;
        gitlab)
            setup_gitlab
            ;;
        jenkins)
            setup_jenkins
            ;;
        all)
            setup_github
            setup_gitlab
            setup_jenkins
            ;;
        *)
            error "Unknown CI system: $CI_SYSTEM"
            ;;
    esac
    
    if [[ "$ENVIRONMENT" == "all" ]]; then
        for env in dev staging production; do
            setup_k8s_resources $env
        done
    else
        setup_k8s_resources $ENVIRONMENT
    fi
    
    setup_monitoring
    
    log "CI/CD setup completed!"
    
    info "Next steps:"
    echo "1. Configure secrets/variables in your CI system"
    echo "2. Set up webhooks for automatic triggers"
    echo "3. Configure branch protection rules"
    echo "4. Set up deployment approvals for production"
    echo "5. Test the pipeline with a sample commit"
}

# Run main function
main