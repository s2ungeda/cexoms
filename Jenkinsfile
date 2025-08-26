pipeline {
    agent {
        kubernetes {
            yaml """
apiVersion: v1
kind: Pod
metadata:
  labels:
    jenkins: agent
spec:
  containers:
  - name: golang
    image: golang:1.21
    tty: true
    resources:
      requests:
        memory: "2Gi"
        cpu: "1"
      limits:
        memory: "4Gi"
        cpu: "2"
  - name: docker
    image: docker:24-dind
    tty: true
    securityContext:
      privileged: true
    volumeMounts:
    - name: docker-sock
      mountPath: /var/run/docker.sock
  - name: kubectl
    image: bitnami/kubectl:1.28
    tty: true
  - name: helm
    image: alpine/helm:3.12.0
    tty: true
  volumes:
  - name: docker-sock
    hostPath:
      path: /var/run/docker.sock
"""
        }
    }
    
    options {
        buildDiscarder(logRotator(numToKeepStr: '30', artifactNumToKeepStr: '10'))
        timeout(time: 1, unit: 'HOURS')
        timestamps()
        skipStagesAfterUnstable()
        disableConcurrentBuilds()
    }
    
    environment {
        DOCKER_REGISTRY = credentials('docker-registry')
        KUBECONFIG = credentials('kubeconfig')
        SLACK_WEBHOOK = credentials('slack-webhook')
        SONAR_TOKEN = credentials('sonar-token')
    }
    
    stages {
        stage('Checkout') {
            steps {
                checkout scm
                script {
                    env.GIT_COMMIT = sh(returnStdout: true, script: 'git rev-parse HEAD').trim()
                    env.GIT_BRANCH = sh(returnStdout: true, script: 'git rev-parse --abbrev-ref HEAD').trim()
                    env.BUILD_TAG = "${env.GIT_BRANCH}-${env.BUILD_NUMBER}-${env.GIT_COMMIT.take(7)}"
                }
            }
        }
        
        stage('Quality Gates') {
            parallel {
                stage('Lint') {
                    steps {
                        container('golang') {
                            sh '''
                                curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.54.2
                                golangci-lint run --timeout=10m ./...
                            '''
                        }
                    }
                }
                
                stage('Security Scan') {
                    steps {
                        container('golang') {
                            sh '''
                                go install github.com/securego/gosec/v2/cmd/gosec@latest
                                gosec -fmt json -out gosec-report.json ./...
                            '''
                            recordIssues(
                                enabledForFailure: true,
                                tools: [goSec(pattern: 'gosec-report.json')]
                            )
                        }
                    }
                }
                
                stage('License Check') {
                    steps {
                        container('golang') {
                            sh '''
                                go install github.com/google/go-licenses@latest
                                go-licenses check ./...
                            '''
                        }
                    }
                }
            }
        }
        
        stage('Build & Test') {
            stages {
                stage('Build C++ Core') {
                    steps {
                        container('golang') {
                            sh '''
                                apt-get update && apt-get install -y cmake g++ libboost-all-dev
                                cd core
                                mkdir -p build && cd build
                                cmake -DCMAKE_BUILD_TYPE=Release ..
                                make -j$(nproc)
                            '''
                        }
                    }
                }
                
                stage('Unit Tests') {
                    steps {
                        container('golang') {
                            sh '''
                                go test -v -race -coverprofile=coverage.out ./...
                                go tool cover -html=coverage.out -o coverage.html
                            '''
                            publishHTML([
                                allowMissing: false,
                                alwaysLinkToLastBuild: true,
                                keepAll: true,
                                reportDir: '.',
                                reportFiles: 'coverage.html',
                                reportName: 'Go Coverage Report'
                            ])
                        }
                    }
                }
                
                stage('Integration Tests') {
                    when {
                        branch pattern: "(main|develop|release/.*)", comparator: "REGEXP"
                    }
                    steps {
                        container('golang') {
                            sh '''
                                docker-compose -f docker-compose.test.yml up -d
                                sleep 30
                                go test -v -tags=integration ./tests/integration/...
                                docker-compose -f docker-compose.test.yml down
                            '''
                        }
                    }
                }
            }
        }
        
        stage('SonarQube Analysis') {
            steps {
                container('golang') {
                    withSonarQubeEnv('SonarQube') {
                        sh '''
                            sonar-scanner \
                                -Dsonar.projectKey=mexoms \
                                -Dsonar.sources=. \
                                -Dsonar.exclusions=**/*_test.go,**/vendor/**,**/testdata/** \
                                -Dsonar.tests=. \
                                -Dsonar.test.inclusions=**/*_test.go \
                                -Dsonar.go.coverage.reportPaths=coverage.out
                        '''
                    }
                }
            }
        }
        
        stage('Build Docker Images') {
            when {
                branch pattern: "(main|develop|release/.*)", comparator: "REGEXP"
            }
            parallel {
                stage('OMS Server') {
                    steps {
                        container('docker') {
                            sh '''
                                docker build -f Dockerfile.oms-server -t ${DOCKER_REGISTRY}/mexoms/oms-server:${BUILD_TAG} .
                                docker push ${DOCKER_REGISTRY}/mexoms/oms-server:${BUILD_TAG}
                            '''
                        }
                    }
                }
                stage('Binance Spot') {
                    steps {
                        container('docker') {
                            sh '''
                                docker build -f Dockerfile.binance-spot -t ${DOCKER_REGISTRY}/mexoms/binance-spot:${BUILD_TAG} .
                                docker push ${DOCKER_REGISTRY}/mexoms/binance-spot:${BUILD_TAG}
                            '''
                        }
                    }
                }
                stage('Binance Futures') {
                    steps {
                        container('docker') {
                            sh '''
                                docker build -f Dockerfile.binance-futures -t ${DOCKER_REGISTRY}/mexoms/binance-futures:${BUILD_TAG} .
                                docker push ${DOCKER_REGISTRY}/mexoms/binance-futures:${BUILD_TAG}
                            '''
                        }
                    }
                }
                stage('Monitor') {
                    steps {
                        container('docker') {
                            sh '''
                                docker build -f Dockerfile.monitor -t ${DOCKER_REGISTRY}/mexoms/monitor:${BUILD_TAG} .
                                docker push ${DOCKER_REGISTRY}/mexoms/monitor:${BUILD_TAG}
                            '''
                        }
                    }
                }
            }
        }
        
        stage('Deploy') {
            when {
                branch pattern: "(main|develop)", comparator: "REGEXP"
            }
            stages {
                stage('Deploy to Dev') {
                    when {
                        branch 'develop'
                    }
                    steps {
                        container('helm') {
                            sh '''
                                helm upgrade --install mexoms-dev ./helm/mexoms \
                                    --namespace mexoms-dev \
                                    --create-namespace \
                                    --values ./helm/mexoms/values-dev.yaml \
                                    --set image.tag=${BUILD_TAG} \
                                    --wait \
                                    --timeout 10m
                            '''
                        }
                    }
                }
                
                stage('Deploy to Staging') {
                    when {
                        branch 'main'
                    }
                    steps {
                        container('helm') {
                            sh '''
                                # Blue-Green deployment
                                helm upgrade --install mexoms-staging-green ./helm/mexoms \
                                    --namespace mexoms-staging \
                                    --create-namespace \
                                    --values ./helm/mexoms/values.yaml \
                                    --set image.tag=${BUILD_TAG} \
                                    --set global.blueGreen.color=green \
                                    --wait \
                                    --timeout 10m
                            '''
                        }
                        
                        container('kubectl') {
                            sh '''
                                # Run smoke tests
                                kubectl run smoke-test --image=curlimages/curl:latest --rm -i --restart=Never -- \
                                    curl -f http://mexoms-staging-green-oms-server:8080/health
                                
                                # Switch traffic
                                kubectl patch ingress mexoms-staging-ingress -n mexoms-staging \
                                    -p '{"spec":{"rules":[{"host":"staging.mexoms.com","http":{"paths":[{"backend":{"service":{"name":"mexoms-staging-green-oms-server"}}}]}}]}}'
                            '''
                        }
                    }
                }
            }
        }
        
        stage('Performance Tests') {
            when {
                branch 'main'
            }
            steps {
                container('golang') {
                    sh '''
                        go test -bench=. -benchmem -run=^$ ./... | tee benchmark.txt
                    '''
                    publishPerformanceReport(
                        sourceDataFiles: 'benchmark.txt',
                        compareBuildPrevious: true,
                        modePerformancePerTestCase: true
                    )
                }
            }
        }
    }
    
    post {
        always {
            container('golang') {
                junit '**/test-report.xml'
                archiveArtifacts artifacts: 'coverage.html,benchmark.txt', fingerprint: true
            }
        }
        success {
            slackSend(
                color: 'good',
                message: "Build Success: ${env.JOB_NAME} - ${env.BUILD_NUMBER} (<${env.BUILD_URL}|Open>)"
            )
        }
        failure {
            slackSend(
                color: 'danger',
                message: "Build Failed: ${env.JOB_NAME} - ${env.BUILD_NUMBER} (<${env.BUILD_URL}|Open>)"
            )
        }
        unstable {
            slackSend(
                color: 'warning',
                message: "Build Unstable: ${env.JOB_NAME} - ${env.BUILD_NUMBER} (<${env.BUILD_URL}|Open>)"
            )
        }
        cleanup {
            cleanWs()
        }
    }
}