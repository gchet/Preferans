import groovy.json.JsonSlurper

plugins { id("com.android.application"); id("org.jetbrains.kotlin.android") }
val sharedLocales = rootProject.file("../locales")
val russianMessages = JsonSlurper().parse(sharedLocales.resolve("ru.json")) as Map<*, *>
val generatedLocaleAssets = layout.buildDirectory.dir("generated/localeAssets")
val copySharedLocales by tasks.registering(Sync::class) {
    from(sharedLocales) { include("*.json"); into("locales") }
    into(generatedLocaleAssets)
}
val preferansVersionCode = providers.gradleProperty("preferansVersionCode").getOrElse("1").toInt()
val preferansVersionName = providers.gradleProperty("preferansVersionName").getOrElse("0.1.0")
android {
    buildFeatures { buildConfig = true }
    namespace = "com.preferans.game"
    compileSdk = 36
    defaultConfig {
        // Отдельный пакет: версия 2.x ставится рядом с 1.x (com.preferans.game), у каждой свои данные и обновления.
        applicationId = "com.preferans.game.v2"
        resValue("string", "app_name", (russianMessages["android.app.name"] as String) + " 2.0")
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
