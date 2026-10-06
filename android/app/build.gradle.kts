import groovy.json.JsonSlurper

plugins { id("com.android.application"); id("org.jetbrains.kotlin.android") }
val sharedLocales = rootProject.file("../locales")
val russianMessages = JsonSlurper().parse(sharedLocales.resolve("ru.json")) as Map<*, *>
val generatedLocaleAssets = layout.buildDirectory.dir("generated/localeAssets")
val copySharedLocales by tasks.registering(Sync::class) {
    from(sharedLocales) { include("*.json"); into("locales") }
    into(generatedLocaleAssets)
}
val networkTest = providers.gradleProperty("networkTest").getOrElse("false").toBoolean()
val preferansVersionCode = providers.gradleProperty("preferansVersionCode").getOrElse("1").toInt()
val preferansVersionName = providers.gradleProperty("preferansVersionName").getOrElse("0.1.0")
android {
    buildFeatures { buildConfig = true }
    namespace = "com.preferans.game"
    compileSdk = 36
    defaultConfig {
        applicationId = "com.preferans.game"
        if (networkTest) { applicationId = "com.preferans.netcheck" }
        buildConfigField("boolean", "NETWORK_TEST", networkTest.toString())
        resValue("string", "app_name", russianMessages[if (networkTest) "android.app.network_name" else "android.app.name"] as String)
        manifestPlaceholders["appLabel"] = "@string/app_name"
        minSdk = 26
        targetSdk = 36
        versionCode = preferansVersionCode
        versionName = preferansVersionName
        ndk { abiFilters += providers.gradleProperty("preferansAbi").getOrElse("arm64-v8a") }
    }
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17; targetCompatibility = JavaVersion.VERSION_17 }
    kotlinOptions { jvmTarget = "17" }
    sourceSets.getByName("main").assets.srcDir(generatedLocaleAssets)
}
tasks.named("preBuild") { dependsOn(copySharedLocales) }
dependencies { implementation(files("libs/preferans.aar")) }
