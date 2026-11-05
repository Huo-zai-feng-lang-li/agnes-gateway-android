import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// 固定 release 签名：优先环境变量（CI），其次 keystore/keystore.properties（本地）
val keystorePropsFile = rootProject.file("keystore/keystore.properties")
val keystoreProps = Properties().apply {
    if (keystorePropsFile.exists()) keystorePropsFile.inputStream().use { load(it) }
}
fun signingValue(envKey: String, propKey: String): String? =
    System.getenv(envKey) ?: keystoreProps.getProperty(propKey)
val keystorePath = signingValue("KEYSTORE_FILE", "storeFile")

android {
    namespace = "com.agnes.gateway"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.agnes.gateway"
        minSdk = 24
        targetSdk = 34
        versionCode = 6
        versionName = "1.0.5"

        ndk {
            abiFilters.add("arm64-v8a")
        }
    }

    signingConfigs {
        val ksPath = keystorePath
        if (ksPath != null) {
            create("release") {
                storeFile = rootProject.file(ksPath)
                storePassword = signingValue("KEYSTORE_PASSWORD", "storePassword")
                keyAlias = signingValue("KEY_ALIAS", "keyAlias")
                keyPassword = signingValue("KEY_PASSWORD", "keyPassword")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            val releaseSigning = signingConfigs.findByName("release")
            if (releaseSigning != null) {
                signingConfig = releaseSigning
            }
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_1_8
        targetCompatibility = JavaVersion.VERSION_1_8
    }

    kotlinOptions {
        jvmTarget = "1.8"
    }

    sourceSets {
        getByName("main") {
            jniLibs.srcDirs("src/main/jniLibs")
        }
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("com.google.android.material:material:1.12.0")
    implementation("androidx.constraintlayout:constraintlayout:2.1.4")
}
