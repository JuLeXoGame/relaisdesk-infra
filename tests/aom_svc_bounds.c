//go:build ignore

#include <stdio.h>
#include <aom/aom_encoder.h>
#include <aom/aomcx.h>

int main(void) {
    aom_codec_ctx_t encoder = {0};
    aom_codec_enc_cfg_t config;
    aom_codec_iface_t *iface = aom_codec_av1_cx();
    if (aom_codec_enc_config_default(iface, &config, AOM_USAGE_REALTIME)) return 1;
    config.g_w = 64;
    config.g_h = 64;
    config.g_threads = 1;
    config.g_lag_in_frames = 0;
    if (aom_codec_enc_init(&encoder, iface, &config, 0)) return 2;
    aom_svc_layer_id_t layer = {0};
    layer.spatial_layer_id = -1;
    if (aom_codec_control(&encoder, AV1E_SET_SVC_LAYER_ID, &layer) != AOM_CODEC_INVALID_PARAM) return 3;
    layer.spatial_layer_id = 0;
    layer.temporal_layer_id = -1;
    if (aom_codec_control(&encoder, AV1E_SET_SVC_LAYER_ID, &layer) != AOM_CODEC_INVALID_PARAM) return 4;
    layer.temporal_layer_id = 0;
    if (aom_codec_control(&encoder, AV1E_SET_SVC_LAYER_ID, &layer) != AOM_CODEC_OK) return 5;
    layer.spatial_layer_id = 100;
    if (aom_codec_control(&encoder, AV1E_SET_SVC_LAYER_ID, &layer) != AOM_CODEC_INVALID_PARAM) return 6;
    if (aom_codec_destroy(&encoder)) return 7;
    puts("AOM SVC bounds: OK");
    return 0;
}
